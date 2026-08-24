package httpapi

import (
	"encoding/json"
	"sync"

	"github.com/coder/websocket"
	"github.com/royal-flush/royal-flush/services/server/internal/room"
)

const roomMembershipRevoked websocket.StatusCode = 4002

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

func (h *realtimeConnections) disconnectUser(userID string, status websocket.StatusCode, reason string) {
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
			_ = connection.Close(status, reason)
		}()
	}
}

func (s *Server) disconnectDepartedUsers(event room.Envelope) {
	if event.Type != "room.player_leaving" && event.Type != "room.player_removed" && event.Type != "room.ended" {
		return
	}
	raw, err := json.Marshal(event.Payload)
	if err != nil {
		return
	}
	var payload struct {
		UserID  string   `json:"userId"`
		UserIDs []string `json:"userIds"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return
	}
	if payload.UserID != "" {
		s.realtime.disconnectUser(payload.UserID, roomMembershipRevoked, "room membership revoked")
	}
	for _, userID := range payload.UserIDs {
		s.realtime.disconnectUser(userID, roomMembershipRevoked, "room membership revoked")
	}
}
