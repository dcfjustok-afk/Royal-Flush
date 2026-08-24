package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/royal-flush/royal-flush/services/server/internal/auth"
	"github.com/royal-flush/royal-flush/services/server/internal/operations"
	"github.com/royal-flush/royal-flush/services/server/internal/room"
	"github.com/royal-flush/royal-flush/services/server/internal/voice"
)

func (s *Server) requestOTP(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Phone string `json:"phone"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	code, expiresAt, err := s.auth.RequestCode(input.Phone)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	response := map[string]any{"expiresAt": expiresAt, "expiresIn": 300}
	if s.config.Development {
		response["devCode"] = code
	}
	writeJSON(writer, http.StatusOK, response)
}

func (s *Server) verifyOTP(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Phone    string `json:"phone"`
		Code     string `json:"code"`
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	user, token, err := s.auth.Verify(request.Context(), input.Phone, input.Code, input.Nickname)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	if err := s.ops.UpsertUser(request.Context(), operations.UserIdentity{ID: user.ID, Phone: user.Phone, Nickname: user.Nickname, CreatedAt: user.CreatedAt}); err != nil {
		writeProblem(writer, http.StatusServiceUnavailable, "identity_store_unavailable", "用户资料暂时无法保存")
		return
	}
	s.scores.EnsureUser(user.ID)
	s.setSessionCookie(writer, token)
	writeJSON(writer, http.StatusOK, map[string]any{"user": user, "balance": s.scores.Balance(user.ID)})
}

func (s *Server) register(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Phone    string `json:"phone"`
		Password string `json:"password"`
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	user, token, err := s.auth.Register(request.Context(), input.Phone, input.Password, input.Nickname)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	if err := s.persistIdentity(request, user); err != nil {
		writeProblem(writer, http.StatusServiceUnavailable, "identity_store_unavailable", "用户资料暂时无法保存")
		return
	}
	s.scores.EnsureUser(user.ID)
	s.setSessionCookie(writer, token)
	writeJSON(writer, http.StatusCreated, map[string]any{"user": user, "balance": s.scores.Balance(user.ID)})
}

func (s *Server) login(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	user, token, err := s.auth.Login(request.Context(), input.Phone, input.Password)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	if err := s.persistIdentity(request, user); err != nil {
		writeProblem(writer, http.StatusServiceUnavailable, "identity_store_unavailable", "用户资料暂时无法保存")
		return
	}
	s.scores.EnsureUser(user.ID)
	s.setSessionCookie(writer, token)
	writeJSON(writer, http.StatusOK, map[string]any{"user": user, "balance": s.scores.Balance(user.ID)})
}

func (s *Server) logout(writer http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie("rf_session"); err == nil {
		user, authenticated, _ := s.auth.UserBySession(request.Context(), cookie.Value)
		if err := s.auth.Logout(request.Context(), cookie.Value); err != nil {
			writeProblem(writer, http.StatusServiceUnavailable, "session_store_unavailable", "暂时无法退出登录")
			return
		}
		if authenticated {
			s.realtime.disconnectUser(user.ID)
		}
	}
	http.SetCookie(writer, &http.Cookie{
		Name: "rf_session", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: !s.config.Development, MaxAge: -1, Expires: time.Unix(1, 0),
	})
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) passwordLogin(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Account  string `json:"account"`
		Password string `json:"password"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	user, token, err := s.auth.PasswordLogin(request.Context(), input.Account, input.Password, s.config.AdminAccount, s.config.AdminPassword)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	if err := s.ops.UpsertUser(request.Context(), operations.UserIdentity{ID: user.ID, Phone: user.Phone, Nickname: user.Nickname, CreatedAt: user.CreatedAt}); err != nil {
		writeProblem(writer, http.StatusServiceUnavailable, "identity_store_unavailable", "用户资料暂时无法保存")
		return
	}
	s.scores.EnsureUser(user.ID)
	s.setSessionCookie(writer, token)
	writeJSON(writer, http.StatusOK, map[string]any{"user": user, "balance": s.scores.Balance(user.ID)})
}

func (s *Server) me(writer http.ResponseWriter, request *http.Request) {
	user := currentUser(request)
	writeJSON(writer, http.StatusOK, map[string]any{"user": user, "balance": s.scores.Balance(user.ID), "activeRoomId": s.rooms.ActiveRoom(user.ID)})
}

func (s *Server) updateMe(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	user, err := s.auth.UpdateNickname(request.Context(), currentUser(request).ID, input.Nickname)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	if err := s.persistIdentity(request, user); err != nil {
		writeProblem(writer, http.StatusServiceUnavailable, "identity_store_unavailable", "用户资料暂时无法保存")
		return
	}
	activeRoomID := s.rooms.ActiveRoom(user.ID)
	if activeRoomID != "" {
		if actor, ok := s.rooms.Room(activeRoomID); ok {
			if err := actor.UpdateIdentity(request.Context(), room.Identity{ID: user.ID, Name: user.Nickname}); err != nil {
				writeProblem(writer, http.StatusServiceUnavailable, "room_profile_unavailable", "昵称已保存，但暂时无法同步到当前房间")
				return
			}
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{"user": user, "balance": s.scores.Balance(user.ID), "activeRoomId": activeRoomID})
}

func (s *Server) persistIdentity(request *http.Request, user auth.User) error {
	return s.ops.UpsertUser(request.Context(), operations.UserIdentity{ID: user.ID, Phone: user.Phone, Nickname: user.Nickname, CreatedAt: user.CreatedAt})
}

func (s *Server) setSessionCookie(writer http.ResponseWriter, token string) {
	http.SetCookie(writer, &http.Cookie{
		Name: "rf_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: !s.config.Development, MaxAge: int((30 * 24 * time.Hour).Seconds()), Expires: time.Now().UTC().Add(30 * 24 * time.Hour),
	})
}

func (s *Server) addScore(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Amount    int64  `json:"amount"`
		RoomID    string `json:"roomId"`
		RequestID string `json:"requestId"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	user := currentUser(request)
	result, err := s.scores.Add(user.ID, input.RoomID, input.RequestID, input.Amount)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	if input.RoomID != "" && s.rooms.ActiveRoom(user.ID) == input.RoomID {
		if actor, ok := s.rooms.Room(input.RoomID); ok {
			_ = actor.BroadcastScoreAddition(request.Context(), user.ID, input.RequestID, input.Amount, result.Balance)
		}
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) scoreLedger(writer http.ResponseWriter, request *http.Request) {
	user := currentUser(request)
	balance, entries := s.scores.Ledger(user.ID)
	writeJSON(writer, http.StatusOK, map[string]any{"balance": balance, "entries": entries})
}

func (s *Server) createRoom(writer http.ResponseWriter, request *http.Request) {
	var config room.Config
	if err := decodeJSON(request, &config); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "房间配置格式不正确")
		return
	}
	user := currentUser(request)
	actor, err := s.rooms.Create(request.Context(), config, room.Identity{ID: user.ID, Name: user.Nickname})
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	snapshot, _ := actor.Snapshot(request.Context(), user.ID)
	writeJSON(writer, http.StatusCreated, map[string]any{"id": actor.ID, "code": actor.Code, "config": snapshot.Config, "snapshot": snapshot})
}

func (s *Server) joinRoom(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Seat int `json:"seat"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "请选择有效座位")
		return
	}
	user := currentUser(request)
	snapshot, err := s.rooms.Join(request.Context(), chi.URLParam(request, "roomID"), room.Identity{ID: user.ID, Name: user.Nickname}, input.Seat)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, snapshot)
}

func (s *Server) publicRoom(writer http.ResponseWriter, request *http.Request) {
	actor, ok := s.rooms.Room(chi.URLParam(request, "roomID"))
	if !ok {
		writeProblem(writer, http.StatusNotFound, "room_not_found", "房间不存在或已结束")
		return
	}
	snapshot, err := actor.PublicSnapshot(request.Context())
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"id": snapshot.RoomID, "code": snapshot.RoomCode, "name": snapshot.RoomName,
		"ownerId": snapshot.OwnerID, "ownerName": ownerName(snapshot), "onlinePlayers": onlinePlayerCount(snapshot.Players), "maxPlayers": snapshot.Config.MaxPlayers,
		"config": snapshot.Config, "occupiedSeats": occupiedSeats(snapshot.Players),
	})
}

func onlinePlayerCount(players []room.PlayerSnapshot) int {
	count := 0
	for _, player := range players {
		if player.Status != "disconnected" {
			count++
		}
	}
	return count
}

func ownerName(snapshot room.TableSnapshot) string {
	for _, player := range snapshot.Players {
		if player.ID == snapshot.OwnerID {
			return player.Name
		}
	}
	return ""
}

func occupiedSeats(players []room.PlayerSnapshot) []int {
	seats := make([]int, 0, len(players))
	for _, player := range players {
		seats = append(seats, player.Seat)
	}
	return seats
}

func (s *Server) roomSnapshot(writer http.ResponseWriter, request *http.Request) {
	actor, ok := s.rooms.Room(chi.URLParam(request, "roomID"))
	if !ok {
		writeProblem(writer, http.StatusNotFound, "room_not_found", "房间不存在或已结束")
		return
	}
	snapshot, err := actor.Snapshot(request.Context(), currentUser(request).ID)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, snapshot)
}

func (s *Server) roomCommand(writer http.ResponseWriter, request *http.Request) {
	actor, ok := s.rooms.Room(chi.URLParam(request, "roomID"))
	if !ok {
		writeProblem(writer, http.StatusNotFound, "room_not_found", "房间不存在或已结束")
		return
	}
	var command room.ClientCommand
	if err := decodeJSON(request, &command); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "牌局命令格式不正确")
		return
	}
	event, duplicate, err := actor.Handle(request.Context(), currentUser(request).ID, command)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"event": event, "duplicate": duplicate})
}

func (s *Server) voiceToken(writer http.ResponseWriter, request *http.Request) {
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
		writeJSON(writer, http.StatusOK, voice.Token{Enabled: false, Reason: "这个房间没有开启语音"})
		return
	}
	token, err := s.config.Voice.Issue(user.ID, user.Nickname, actor.ID, time.Now())
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, token)
}

func (s *Server) resetScores(writer http.ResponseWriter, request *http.Request) {
	user := currentUser(request)
	if !user.Has("score:reset-all") {
		writeProblem(writer, http.StatusForbidden, "missing_permission", "需要 score:reset-all 权限")
		return
	}
	var input struct {
		Reason       string `json:"reason"`
		Confirmation string `json:"confirmation"`
		RequestID    string `json:"requestId"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "重置请求格式不正确")
		return
	}
	if input.Confirmation != "RESET ALL SCORES" || input.RequestID == "" {
		writeProblem(writer, http.StatusBadRequest, "confirmation_required", "请输入 RESET ALL SCORES 并提供 requestId")
		return
	}
	epoch, duplicate, err := s.scores.ResetAllWithRequest(user.ID, input.Reason, input.RequestID)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	if !duplicate {
		s.rooms.BroadcastGlobalReset(request.Context(), epoch, input.RequestID)
	}
	writeJSON(writer, http.StatusOK, map[string]any{"epoch": epoch.ID, "baseScore": epoch.BaseScore, "audit": epoch, "duplicate": duplicate})
}

func (s *Server) scoreEpochs(writer http.ResponseWriter, request *http.Request) {
	user := currentUser(request)
	if !user.Has("admin:read") {
		writeProblem(writer, http.StatusForbidden, "missing_permission", "需要 admin:read 权限")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"epochs": s.scores.Epochs()})
}
