// +build ignore
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", "localhost:8162", "http service address")

var upgrader = websocket.Upgrader{} // use default options

func echo(w http.ResponseWriter, r *http.Request) {
	scanner := bufio.NewScanner(os.Stdin)
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)

		var task struct {
			Type    string `json:"type"`
			Content struct {
				URL string `json:"url"`
			} `json:"content"`
		}

		scanner.Scan()
		task.Type = "task"
		task.Content.URL = scanner.Text()
		b, _ := json.Marshal(&task)
		err = c.WriteMessage(
			websocket.TextMessage,
			b,
		)
		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	http.HandleFunc("/", echo)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
