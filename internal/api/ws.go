package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// upgrader promotes HTTP requests to WebSocket connections.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return allowedOrigin(r.Header.Get("Origin"), r.Host)
	},
}

// allowedOrigin accepts same-origin requests and explicitly permits the
// loopback origins commonly used by the local development UI.
//
// No wildcard origin is accepted.
func allowedOrigin(origin, host string) bool {
	origin = strings.TrimSpace(origin)

	// Browsers normally omit Origin for same-origin WebSocket requests.
	if origin == "" {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	originHost := strings.ToLower(u.Host)
	requestHost := strings.ToLower(strings.TrimSpace(host))

	// Same-origin request.
	if requestHost != "" && originHost == requestHost {
		return true
	}

	// Local development frontends.
	switch originHost {
	case "localhost:3000",
		"127.0.0.1:3000",
		"localhost:4173",
		"127.0.0.1:4173",
		"localhost:5173",
		"127.0.0.1:5173":
		return isLoopbackHost(requestHost)
	default:
		return false
	}
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(host)

	return strings.HasPrefix(host, "localhost:") ||
		strings.HasPrefix(host, "127.0.0.1:") ||
		host == "localhost" ||
		host == "127.0.0.1"
}
