package websocket

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/pizixi/goprocess/internal/models"
)

var Upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var Clients = make(map[*websocket.Conn]bool)
var Broadcast = make(chan models.RuntimeProcess)
var clientsMu sync.Mutex

func init() {
	go HandleMessages()
}
func HandleWebSocket(c echo.Context) error {
	ws, err := Upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	clientsMu.Lock()
	Clients[ws] = true
	clientsMu.Unlock()
	defer removeClient(ws)

	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			log.Println(err)
			return nil
		}
	}
}

func BroadcastStatus(rp models.RuntimeProcess) {
	Broadcast <- rp
}

func HandleMessages() {
	for {
		rp := <-Broadcast
		clientsMu.Lock()
		clients := make([]*websocket.Conn, 0, len(Clients))
		for client := range Clients {
			clients = append(clients, client)
		}
		clientsMu.Unlock()

		for _, client := range clients {
			err := client.WriteJSON(rp)
			if err != nil {
				log.Printf("error: %v", err)
				removeClient(client)
			}
		}
	}
}

func removeClient(client *websocket.Conn) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	if Clients[client] {
		client.Close()
		delete(Clients, client)
	}
}
