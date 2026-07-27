// Package wsfallback provides a WebSocket-based audio relay fallback for
// clients that cannot use WebRTC (Req 4.2, 3.5).
//
// Each tenant gets one Hub. The broadcaster writes raw Opus frames as binary
// WebSocket messages; the Hub fans each frame out to all registered listeners.
// Listeners with a write error are silently removed.
package wsfallback

import (
	"context"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

// wsConn wraps a *websocket.Conn so we can use it as a map key via a pointer.
type wsConn struct {
	conn *websocket.Conn
}

// Hub is a per-tenant fan-out hub for WebSocket-based Opus frames.
type Hub struct {
	broadcast   chan []byte
	register    chan *wsConn
	unregister  chan *wsConn
	listeners   map[*wsConn]struct{}
	mu          sync.Mutex
	done        chan struct{}
}

// NewHub creates a Hub and starts its internal fan-out goroutine.
func NewHub() *Hub {
	h := &Hub{
		broadcast:  make(chan []byte, 256),
		register:   make(chan *wsConn, 64),
		unregister: make(chan *wsConn, 64),
		listeners:  make(map[*wsConn]struct{}),
		done:       make(chan struct{}),
	}
	go h.run()
	return h
}

// Stop shuts down the hub's goroutine.
func (h *Hub) Stop() {
	close(h.done)
}

// Broadcast sends a binary Opus frame to all registered listeners.
// Non-blocking: drops the frame if the internal channel is full.
func (h *Hub) Broadcast(frame []byte) {
	select {
	case h.broadcast <- frame:
	default:
	}
}

// ServeListener upgrades the HTTP connection to a WebSocket, registers it with
// the hub, and blocks until the connection closes.
func (h *Hub) ServeListener(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: false,
	})
	if err != nil {
		return
	}

	wc := &wsConn{conn: conn}
	h.register <- wc
	defer func() { h.unregister <- wc }()

	// Keep-alive read loop — we don't expect messages from listeners, but we
	// need to drain the connection to detect disconnects.
	for {
		_, _, err := conn.Read(r.Context())
		if err != nil {
			return
		}
	}
}

// run is the hub's main goroutine.
func (h *Hub) run() {
	for {
		select {
		case <-h.done:
			return

		case wc := <-h.register:
			h.mu.Lock()
			h.listeners[wc] = struct{}{}
			h.mu.Unlock()

		case wc := <-h.unregister:
			h.mu.Lock()
			delete(h.listeners, wc)
			h.mu.Unlock()
			wc.conn.Close(websocket.StatusNormalClosure, "bye")

		case frame := <-h.broadcast:
			h.mu.Lock()
			listeners := make([]*wsConn, 0, len(h.listeners))
			for wc := range h.listeners {
				listeners = append(listeners, wc)
			}
			h.mu.Unlock()

			var dead []*wsConn
			for _, wc := range listeners {
				if err := wc.conn.Write(context.Background(), websocket.MessageBinary, frame); err != nil {
					dead = append(dead, wc)
				}
			}

			if len(dead) > 0 {
				h.mu.Lock()
				for _, wc := range dead {
					delete(h.listeners, wc)
					wc.conn.Close(websocket.StatusNormalClosure, "")
				}
				h.mu.Unlock()
			}
		}
	}
}
