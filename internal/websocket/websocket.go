package websocket

import (
	"log"
	"net/http"

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

func init() {
	go HandleMessages()
}
func HandleWebSocket(c echo.Context) error {
	ws, err := Upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	Clients[ws] = true

	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			log.Println(err)
			delete(Clients, ws)
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
		for client := range Clients {
			err := client.WriteJSON(rp)
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
				delete(Clients, client)
			}
		}
	}
}
