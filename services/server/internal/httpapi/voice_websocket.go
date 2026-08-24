package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"
	"github.com/royal-flush/royal-flush/services/server/internal/auth"
)

const maxVoiceSignalBytes = 64 << 10

type voicePeer struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname"`
}

type voiceClientMessage struct {
	Type         string          `json:"type"`
	TargetUserID string          `json:"targetUserId"`
	Payload      json.RawMessage `json:"payload"`
}

type voiceServerEvent struct {
	Type       string          `json:"type"`
	UserID     string          `json:"userId,omitempty"`
	Nickname   string          `json:"nickname,omitempty"`
	FromUserID string          `json:"fromUserId,omitempty"`
	SignalType string          `json:"signalType,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Peers      []voicePeer     `json:"peers,omitempty"`
	Message    string          `json:"message,omitempty"`
}

type voiceClient struct {
	roomID string
	user   auth.User
	send   chan voiceServerEvent
}

type voiceHub struct {
	mu    sync.Mutex
	rooms map[string]map[string]map[*voiceClient]struct{}
}

func newVoiceHub() *voiceHub {
	return &voiceHub{rooms: map[string]map[string]map[*voiceClient]struct{}{}}
}

func (h *voiceHub) join(client *voiceClient) []voicePeer {
	h.mu.Lock()
	defer h.mu.Unlock()
	roomClients := h.rooms[client.roomID]
	if roomClients == nil {
		roomClients = map[string]map[*voiceClient]struct{}{}
		h.rooms[client.roomID] = roomClients
	}
	peers := make([]voicePeer, 0, len(roomClients))
	for userID, connections := range roomClients {
		if userID == client.user.ID || len(connections) == 0 {
			continue
		}
		for connection := range connections {
			peers = append(peers, voicePeer{UserID: userID, Nickname: connection.user.Nickname})
			break
		}
	}
	connections := roomClients[client.user.ID]
	firstConnection := len(connections) == 0
	if connections == nil {
		connections = map[*voiceClient]struct{}{}
		roomClients[client.user.ID] = connections
	}
	connections[client] = struct{}{}
	if firstConnection {
		h.broadcastLocked(client.roomID, client.user.ID, voiceServerEvent{Type: "voice.peer_joined", UserID: client.user.ID, Nickname: client.user.Nickname})
	}
	return peers
}

func (h *voiceHub) leave(client *voiceClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	roomClients := h.rooms[client.roomID]
	connections := roomClients[client.user.ID]
	delete(connections, client)
	if len(connections) > 0 {
		return
	}
	delete(roomClients, client.user.ID)
	h.broadcastLocked(client.roomID, client.user.ID, voiceServerEvent{Type: "voice.peer_left", UserID: client.user.ID})
	if len(roomClients) == 0 {
		delete(h.rooms, client.roomID)
	}
}

func (h *voiceHub) relay(client *voiceClient, message voiceClientMessage) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	targets := h.rooms[client.roomID][message.TargetUserID]
	if len(targets) == 0 {
		return false
	}
	event := voiceServerEvent{Type: "voice.signal", FromUserID: client.user.ID, SignalType: message.Type, Payload: message.Payload}
	for target := range targets {
		select {
		case target.send <- event:
		default:
		}
	}
	return true
}

func (h *voiceHub) broadcastLocked(roomID, exceptUserID string, event voiceServerEvent) {
	for userID, connections := range h.rooms[roomID] {
		if userID == exceptUserID {
			continue
		}
		for client := range connections {
			select {
			case client.send <- event:
			default:
			}
		}
	}
}

func (s *Server) voiceEvents(writer http.ResponseWriter, request *http.Request) {
	actor, ok := s.rooms.Room(chi.URLParam(request, "roomID"))
	if !ok {
		writeProblem(writer, http.StatusNotFound, "room_not_found", "房间不存在或已结束")
		return
	}
	user := currentUser(request)
	snapshot, err := actor.Snapshot(request.Context(), user.ID)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	if !snapshot.Config.VoiceEnabled {
		writeProblem(writer, http.StatusForbidden, "voice_disabled", "这个房间没有开启语音")
		return
	}
	connection, err := websocket.Accept(writer, request, &websocket.AcceptOptions{OriginPatterns: s.websocketOrigins()})
	if err != nil {
		return
	}
	defer connection.CloseNow()
	connection.SetReadLimit(maxVoiceSignalBytes)
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	unregister := s.realtime.add(user.ID, connection)
	defer unregister()
	client := &voiceClient{roomID: actor.ID, user: user, send: make(chan voiceServerEvent, 32)}
	peers := s.voiceHub.join(client)
	defer s.voiceHub.leave(client)
	if err := wsjson.Write(ctx, connection, voiceServerEvent{Type: "voice.peers", Peers: peers}); err != nil {
		return
	}
	read := make(chan voiceClientMessage)
	readErrors := make(chan error, 1)
	go func() {
		defer close(read)
		for {
			var message voiceClientMessage
			if err := wsjson.Read(ctx, connection, &message); err != nil {
				readErrors <- err
				return
			}
			select {
			case read <- message:
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		select {
		case event := <-client.send:
			if wsjson.Write(ctx, connection, event) != nil {
				return
			}
		case message, open := <-read:
			if !open {
				return
			}
			if (message.Type != "voice.description" && message.Type != "voice.candidate") || message.TargetUserID == "" || message.TargetUserID == user.ID || len(message.Payload) == 0 || !json.Valid(message.Payload) {
				if wsjson.Write(ctx, connection, voiceServerEvent{Type: "voice.error", Message: "语音信令格式不正确"}) != nil {
					return
				}
				continue
			}
			if !s.voiceHub.relay(client, message) {
				if wsjson.Write(ctx, connection, voiceServerEvent{Type: "voice.error", Message: "对方语音连接已离开"}) != nil {
					return
				}
			}
		case err := <-readErrors:
			if !errors.Is(err, context.Canceled) && websocket.CloseStatus(err) == -1 {
				s.log.Debug("voice websocket reader stopped", "error", err)
			}
			return
		case <-ctx.Done():
			return
		}
	}
}
