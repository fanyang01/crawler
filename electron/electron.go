package electron

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/fanyang01/crawler"
	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
	"github.com/nats-io/nats"
)

type requestMsg struct {
	TaskID    uint32      `json:"taskID"`
	Timeout   int         `json:"timeout"` // in milliseconds
	URL       string      `json:"url"`
	Mode      string      `json:"mode,omitempty"`
	FetchCode string      `json:"fetchCode,omitempty"`
	Injection string      `json:"injection,omitempty"`
	Headers   http.Header `json:"headers,omitempty"`
	Proxy     string      `json:"proxy,omitempty"`
	Cookies   []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"cookies,omitempty"`
}

func reqToMsg(req *crawler.Request) *requestMsg {
	m := &requestMsg{
		URL:     req.URL.String(),
		Headers: req.Header,
		Proxy:   req.Proxy.String(),
	}
	config := req.BrowserConfig
	if config != nil {
		m.Mode = config.Mode
		m.Timeout = int(config.Timeout / time.Millisecond)
		m.FetchCode = m.FetchCode
		m.Injection = m.Injection
	}
	for _, cookie := range req.Cookies {
		m.Cookies = append(m.Cookies, struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}{Name: cookie.Name, Value: cookie.Value})
	}
	return m
}

type responseMsg struct {
	TaskID      uint32      `json:"taskID" mapstructure:"taskID"`
	NewURL      string      `json:"newURL" mapstructure:"newURL"`
	OriginalURL string      `json:"originalURL" mapstructure:"originalURL"`
	StatusCode  int         `json:"statusCode" mapstructure:"statusCode"`
	Content     []byte      `json:"content" mapstructure:"content"`
	ContentType string      `json:"contentType" mapstructure:"contentType"`
	Headers     http.Header `json:"headers" mapstructure:"headers"`
	Cookies     []struct {
		Name  string `json:"name" mapstructure:"name"`
		Value string `json:"value" mapstructure:"value"`
	} `json:"cookies" mapstructure:"cookies"`
}

func msgToResp(msg *responseMsg) *crawler.Response {
	r := &http.Response{
		Status:     http.StatusText(msg.StatusCode),
		StatusCode: msg.StatusCode,
		Proto:      "HTTP/1.0",
		ProtoMajor: 1,
		ProtoMinor: 0,
		Header:     msg.Headers,
		Request: &http.Request{
			Method: "GET",
		},
	}
	if u, err := url.Parse(msg.OriginalURL); err == nil {
		r.Request.URL = u
	}
	if r.Header == nil {
		r.Header = http.Header{}
	}
	if msg.ContentType != "" {
		r.Header.Set("Content-Type", msg.ContentType)
	}
	return &crawler.Response{
		Response:   r,
		Body:       bytes.NewReader(msg.Content),
		Content:    msg.Content,
		ClientType: crawler.CLIENT_BROWSER,
	}
}

// ElectronConn connects to Electron instance(s) through NATS.
type ElectronConn struct {
	conn     *nats.Conn
	jsonConn *nats.EncodedConn
	clients  []uint32
	genID    uint32
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
		ID := atomic.AddUint32(&ec.genID, 1)
		if err := nc.Publish(m.Reply, []byte(fmt.Sprintf("%d", ID))); err != nil {
			logrus.Error(err)
			return
		}
		ec.Lock()
		ec.clients = append(ec.clients, ID)
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

func (ec *ElectronConn) Do(req *crawler.Request) (resp *crawler.Response, err error) {
	request := reqToMsg(req)
	var response responseMsg
	if err = ec.jsonConn.Request("job", request, &response, 20*time.Second); err != nil {
		return
	}
	return msgToResp(&response), nil
}

type ewJob struct {
	req   *crawler.Request
	reply chan *crawler.Response // reply channel must be buffered if the reply may be ignored.
}

// ElectronWebsocket connects to Electron instance(s) through Websocket.
type ElectronWebsocket struct {
	N, M      int
	clients   []uint32
	genID     uint32
	worker    chan chan *ewJob // chan *ewJob must be buffered.
	reply     map[uint32]map[uint32]chan *crawler.Response
	genTaskID uint32
	sync.Mutex
}

func NewElectronWebsocket(URL string, N, M int) (ew *ElectronWebsocket, err error) {
	var u *url.URL
	if u, err = url.Parse(URL); err != nil {
		return
	}
	l, err := net.Listen("tcp", u.Host)
	if err != nil {
		return nil, err
	}
	ew = &ElectronWebsocket{
		N:      N,
		M:      M,
		worker: make(chan chan *ewJob, N*M),
		reply:  make(map[uint32]map[uint32]chan *crawler.Response),
	}
	go func() {
		http.Serve(l, http.HandlerFunc(ew.handle))
	}()
	return
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
	defer func() {
		c.Close()
		close(quit)
	}()
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

	chJob := make(chan *ewJob, 1)
	defer close(chJob)
	tick := time.Tick(10 * time.Second)
	var ID uint32
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
					ID = atomic.AddUint32(&ew.genID, 1)
					if err := c.WriteMessage(
						websocket.TextMessage, []byte(fmt.Sprintf("%d", ID)),
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
						TaskID uint32 `mapstructure:"taskID"`
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

func (ew *ElectronWebsocket) storeClient(id uint32) {
	ew.Lock()
	defer ew.Unlock()
	ew.clients = append(ew.clients, id)
	ew.reply[id] = make(map[uint32]chan *crawler.Response, ew.M)
}

func (ew *ElectronWebsocket) removeClient(id uint32) {
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

func (ew *ElectronWebsocket) nextTask(id uint32,
	reply chan *crawler.Response, chJob chan *ewJob) uint32 {

	taskID := atomic.AddUint32(&ew.genTaskID, 1)
	ew.Lock()
	defer ew.Unlock()
	ew.reply[id][taskID] = reply
	if len(ew.reply[id]) < ew.M {
		ew.worker <- chJob
	}
	return taskID
}

func (ew *ElectronWebsocket) sendReply(id uint32, resp *responseMsg) {
	ew.Lock()
	defer ew.Unlock()
	reply := ew.reply[id][resp.TaskID]
	if reply == nil {
		return
	}
	reply <- msgToResp(resp)
	delete(ew.reply[id], resp.TaskID)
}

func (ew *ElectronWebsocket) cancelReply(id, taskID uint32) {
	ew.Lock()
	defer ew.Unlock()
	reply := ew.reply[id][taskID]
	if reply == nil {
		return
	}
	close(reply)
	delete(ew.reply[id], taskID)
}

func (ew *ElectronWebsocket) Do(req *crawler.Request) (resp *crawler.Response, err error) {
	// quick method to determine if there are available clients, but it's unsafe.
	var nclient int
	ew.Lock()
	nclient = len(ew.clients)
	ew.Unlock()
	if nclient == 0 {
		return nil, errors.New("no available websocket client")
	}

	timeout := time.After(20 * time.Second)
	ch := make(chan *crawler.Response, 1)
	job := &ewJob{
		req:   req,
		reply: ch,
	}
	var w chan *ewJob
LOOP:
	for {
		select {
		case w = <-ew.worker:
			select {
			case w <- job:
				break LOOP
			default:
				continue LOOP
			}
		case <-timeout:
			return nil, errors.New("timeout: no available client")
		}
	}
	select {
	case resp = <-ch:
		if resp == nil {
			return nil, errors.New("request canceled by (broken) client")
		}
	case <-timeout:
		return nil, errors.New("client timeout")
	}
	resp.URL = req.URL
	return
}
