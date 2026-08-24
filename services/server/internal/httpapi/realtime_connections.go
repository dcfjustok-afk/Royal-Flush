package httpapi

import (
	"sync"

	"github.com/coder/websocket"
)

type realtimeConnections struct {
	mu     sync.Mutex
	byUser map[string]map[*websocket.Conn]struct{}
}

func newRealtimeConnections() *realtimeConnections {
	return &realtimeConnections{byUser: make(map[string]map[*websocket.Conn]struct{})}
}

func (h *realtimeConnections) add(userID string, connection *websocket.Conn) func() {
	h.mu.Lock()
	connections := h.byUser[userID]
	if connections == nil {
		connections = make(map[*websocket.Conn]struct{})
		h.byUser[userID] = connections
	}
	connections[connection] = struct{}{}
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		connections := h.byUser[userID]
		delete(connections, connection)
		if len(connections) == 0 {
			delete(h.byUser, userID)
		}
		h.mu.Unlock()
	}
}

func (h *realtimeConnections) disconnectUser(userID string) {
	h.mu.Lock()
	connections := make([]*websocket.Conn, 0, len(h.byUser[userID]))
	for connection := range h.byUser[userID] {
		connections = append(connections, connection)
	}
	delete(h.byUser, userID)
	h.mu.Unlock()
	for _, connection := range connections {
		connection := connection
		go func() {
			_ = connection.Close(websocket.StatusPolicyViolation, "account session revoked")
		}()
	}
}
