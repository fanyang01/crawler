package crawler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
	"github.com/nats-io/nats"
)

// ElectronConn connects to Electron instance(s) through NATS.
type ElectronConn struct {
	conn     *nats.Conn
	jsonConn *nats.EncodedConn
	clients  []int32
	genID    int32
	sync.Mutex
}

func NewElectronConn(opt *nats.Options) (ec *ElectronConn, err error) {
	var nc *nats.Conn
	if opt == nil {
		nc, err = nats.Connect(nats.DefaultURL)
	} else {
		nc, err = opt.Connect()
	}
	if err != nil {
		return nil, err
	}

	ec = &ElectronConn{conn: nc}
	f := func(m *nats.Msg) {
		nextID := atomic.AddInt32(&ec.genID, 1)
		if err := nc.Publish(m.Reply, []byte(strconv.Itoa(int(nextID)))); err != nil {
			logrus.Error(err)
			return
		}
		ec.Lock()
		ec.clients = append(ec.clients, nextID)
		ec.Unlock()
	}
	if _, err = nc.Subscribe("register", f); err != nil {
		nc.Close()
		return
	}

	if ec.jsonConn, err = nats.NewEncodedConn(nc, "json"); err != nil {
		nc.Close()
		return nil, err
	}
	return
}

func (ec *ElectronConn) Do(req *Request) (resp *Response, err error) {
	request := reqToMsg(req)
	var response responseMsg
	if err = ec.jsonConn.Request("job", request, &response, 20*time.Second); err != nil {
		return
	}
	return msgToResp(&response), nil
}

type requestMsg struct {
	TaskID  int32       `json:"taskID"`
	URL     string      `json:"url,omitempty"`
	Headers http.Header `json:"headers,omitempty"`
	Proxy   string      `json:"proxy,omitempty"`
	Cookies []struct {
		Name  string `json:"name,omitempty"`
		Value string `json:"value,omitempty"`
	} `json:"cookies,omitempty"`
}

func reqToMsg(req *Request) *requestMsg {
	m := &requestMsg{
		URL:     req.URL.String(),
		Headers: req.Header,
		Proxy:   req.Proxy.String(),
	}
	for _, cookie := range req.Cookies {
		m.Cookies = append(m.Cookies, struct {
			Name  string `json:"name,omitempty"`
			Value string `json:"value,omitempty"`
		}{Name: cookie.Name, Value: cookie.Value})
	}
	return m
}

type responseMsg struct {
	TaskID        int32       `json:"taskID"`
	NewURL        string      `json:"newURL"`
	OriginalURL   string      `json:"originalURL"`
	RequestMethod string      `json:"requestMethod"`
	StatusCode    int         `json:"statusCode,omitempty"`
	Content       []byte      `json:"content,omitempty"`
	Headers       http.Header `json:"headers"`
	Cookies       []struct {
		Name  string `json:"name,omitempty"`
		Value string `json:"value,omitempty"`
	} `json:"cookies,omitempty"`
}

func msgToResp(msg *responseMsg) *Response {
	r := &http.Response{
		Status:     http.StatusText(msg.StatusCode),
		StatusCode: msg.StatusCode,
		Proto:      "HTTP/1.0",
		ProtoMajor: 1,
		ProtoMinor: 0,
		Header:     msg.Headers,
		Request: &http.Request{
			Method: msg.RequestMethod,
		},
	}
	if u, err := url.Parse(msg.OriginalURL); err == nil {
		r.Request.URL = u
	}
	if r.Header == nil {
		r.Header = http.Header{}
	}
	if r.Header.Get("Location") == "" {
		r.Header.Set("Location", msg.NewURL)
	}
	return &Response{
		Response:   r,
		Content:    msg.Content,
		BodyStatus: BodyStatusEOF,
	}
}

type ewJob struct {
	req   *Request
	reply chan *Response // reply channel must be buffered if the reply may be ignored.
}

// ElectronWebsocket connects to Electron instance(s) through Websocket.
type ElectronWebsocket struct {
	N, M      int
	clients   []int32
	genID     int32
	worker    chan chan *ewJob
	reply     map[int32]map[int32]chan *Response
	genTaskID int32
	sync.Mutex
}

func NewElectronWebsocket(addr string, N, M int) (*ElectronWebsocket, error) {
	return &ElectronWebsocket{
		N:      N,
		M:      M,
		worker: make(chan chan *ewJob, N*M),
		reply:  make(map[int32]map[int32]chan *Response),
	}, nil
}

var upgrader = websocket.Upgrader{}

func (ew *ElectronWebsocket) handle(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logrus.Error("[websocket]", err)
	}
	type msgRead struct {
		typ int
		msg []byte
	}
	ch := make(chan *msgRead, ew.M)
	quit := make(chan struct{})
	go func() {
		defer close(ch)
		for {
			mt, message, err := c.ReadMessage()
			if err != nil {
				logrus.Error("[websocket] read:", err)
				return
			}
			m := &msgRead{typ: mt, msg: message}
			select {
			case ch <- m:
			case <-quit:
				return
			}
		}
	}()
	defer func() {
		c.Close()
		close(quit)
	}()

	chJob := make(chan *ewJob, 1)
	defer close(chJob)
	tick := time.Tick(10 * time.Second)
	var ID int32
	for {
		select {
		case m := <-ch:
			if m == nil { // ch was closed
				return
			}
			switch m.typ {
			case websocket.TextMessage:
				var msg struct {
					Typ     string      `json:"type"`
					Content interface{} `json:"content"`
				}
				if err := json.Unmarshal(m.msg, &msg); err != nil {
					logrus.Error(err)
					return
				}
				switch msg.Typ {
				case "init":
					ID = atomic.AddInt32(&ew.genID, 1)
					if err := c.WriteMessage(
						websocket.TextMessage, []byte(strconv.Itoa(int(ID))),
					); err != nil {
						logrus.Error("[websocket] write:", err)
						return
					}
					ew.storeClient(ID)
					ew.worker <- chJob
					// I know it's in the for loop...
					defer ew.removeClient(ID)
				case "task":
					var response responseMsg
					if err := mapstructure.Decode(msg.Content, &response); err != nil {
						logrus.Error(err)
						return
					}
					ew.sendReply(ID, &response)
					ew.worker <- chJob
				case "timeout":
					var response struct {
						TaskID int32 `mapstructure:"taskID"`
					}
					if err := mapstructure.Decode(msg.Content, &response); err != nil {
						logrus.Error(err)
						return
					}
					ew.cancelReply(ID, response.TaskID)
					ew.worker <- chJob
				}
			}
		case job := <-chJob:
			req := reqToMsg(job.req)
			req.TaskID = ew.nextTask(ID, job.reply, chJob)
			if err := c.WriteJSON(req); err != nil {
				logrus.Error("[websocket] write:", err)
				return
			}
		case <-tick:
			if err := c.WriteMessage(
				websocket.PingMessage, []byte("ping"),
			); err != nil {
				logrus.Error("[websocket] write:", err)
				return
			}
		}
	}
}

func (ew *ElectronWebsocket) storeClient(id int32) {
	ew.Lock()
	defer ew.Unlock()
	ew.clients = append(ew.clients, id)
	ew.reply[id] = make(map[int32]chan *Response, ew.M)
}

func (ew *ElectronWebsocket) removeClient(id int32) {
	ew.Lock()
	defer ew.Unlock()
	for i := 0; i < len(ew.clients); i++ {
		if ew.clients[i] == id {
			ew.clients = append(ew.clients[:i], ew.clients[i+1:]...)
			break
		}
	}
	delete(ew.reply, id)
}

func (ew *ElectronWebsocket) nextTask(id int32,
	reply chan *Response, chJob chan *ewJob) int32 {

	taskID := atomic.AddInt32(&ew.genTaskID, 1)
	ew.Lock()
	defer ew.Unlock()
	ew.reply[id][taskID] = reply
	if len(ew.reply[id]) < ew.M {
		ew.worker <- chJob
	}
	return taskID
}

func (ew *ElectronWebsocket) sendReply(id int32, resp *responseMsg) {
	ew.Lock()
	defer ew.Unlock()
	reply := ew.reply[id][resp.TaskID]
	if reply == nil {
		return
	}
	reply <- msgToResp(resp)
	delete(ew.reply[id], resp.TaskID)
}

func (ew *ElectronWebsocket) cancelReply(id, taskID int32) {
	ew.Lock()
	defer ew.Unlock()
	reply := ew.reply[id][taskID]
	if reply == nil {
		return
	}
	close(reply)
	delete(ew.reply[id], taskID)
}

func (ew *ElectronWebsocket) Do(req *Request) (resp *Response, err error) {
	w := <-ew.worker
	ch := make(chan *Response)
	w <- &ewJob{
		req:   req,
		reply: ch,
	}
	timeout := time.After(20 * time.Second)
	select {
	case resp = <-ch:
		if resp == nil {
			return nil, errors.New("canceled by (broken) client")
		}
	case <-timeout:
		return nil, errors.New("client timeout")
	}
	resp.URL = req.URL
	resp.parseLocation()
	return
}
