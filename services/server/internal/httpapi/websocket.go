package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"
	"github.com/royal-flush/royal-flush/services/server/internal/room"
)

func (s *Server) roomEvents(writer http.ResponseWriter, request *http.Request) {
	actor, ok := s.rooms.Room(chi.URLParam(request, "roomID"))
	if !ok {
		writeProblem(writer, http.StatusNotFound, "room_not_found", "房间不存在或已结束")
		return
	}
	user := currentUser(request)
	if _, err := actor.Snapshot(request.Context(), user.ID); err != nil {
		writeDomainError(writer, err)
		return
	}
	connection, err := websocket.Accept(writer, request, &websocket.AcceptOptions{OriginPatterns: s.websocketOrigins()})
	if err != nil {
		return
	}
	defer connection.CloseNow()
	connection.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	unregister := s.realtime.add(user.ID, connection)
	defer unregister()
	if err := actor.PlayerConnected(ctx, user.ID); err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "player is not seated")
		return
	}
	defer func() { _ = actor.PlayerDisconnected(context.Background(), user.ID) }()
	snapshot, err := actor.Snapshot(ctx, user.ID)
	if err != nil {
		_ = connection.Close(websocket.StatusInternalError, "snapshot unavailable")
		return
	}
	events, unsubscribe, err := actor.Subscribe(ctx)
	if err != nil {
		_ = connection.Close(websocket.StatusInternalError, "subscription failed")
		return
	}
	defer unsubscribe()
	initial := room.Envelope{Type: "table.snapshot", RoomID: actor.ID, Version: snapshot.Version, Payload: snapshot}
	if err := wsjson.Write(ctx, connection, initial); err != nil {
		return
	}
	commands := make(chan room.ClientCommand)
	readErrors := make(chan error, 1)
	go func() {
		defer close(commands)
		for {
			var command room.ClientCommand
			if err := wsjson.Read(ctx, connection, &command); err != nil {
				readErrors <- err
				return
			}
			select {
			case commands <- command:
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		select {
		case event, open := <-events:
			if !open || wsjson.Write(ctx, connection, event) != nil {
				return
			}
		case command, open := <-commands:
			if !open {
				return
			}
			event, duplicate, err := actor.Handle(ctx, user.ID, command)
			if err != nil {
				status, code, message := domainErrorProblem(err)
				if status == http.StatusInternalServerError {
					s.log.Error("unhandled WebSocket domain error", "error", err)
				}
				problem := room.Envelope{Type: "error", RequestID: command.RequestID, RoomID: actor.ID, Version: snapshot.Version, Payload: map[string]any{"code": code, "message": message}}
				if wsjson.Write(ctx, connection, problem) != nil {
					return
				}
				continue
			}
			s.disconnectDepartedUsers(event)
			if duplicate && wsjson.Write(ctx, connection, event) != nil {
				return
			}
		case err := <-readErrors:
			if !errors.Is(err, context.Canceled) && websocket.CloseStatus(err) == -1 {
				s.log.Debug("websocket reader stopped", "error", err)
			}
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) websocketOrigins() []string {
	if len(s.config.AllowedOrigins) > 0 {
		return append([]string(nil), s.config.AllowedOrigins...)
	}
	if s.config.Development {
		return []string{"localhost:*", "127.0.0.1:*"}
	}
	return nil
}
