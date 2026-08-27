package api

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// upgrader promotes HTTP requests to WebSocket connections.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}
