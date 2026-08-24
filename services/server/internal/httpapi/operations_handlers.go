package httpapi

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/royal-flush/royal-flush/services/server/internal/auth"
	"github.com/royal-flush/royal-flush/services/server/internal/operations"
	"github.com/royal-flush/royal-flush/services/server/internal/room"
)

func (s *Server) createReport(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		RoomID        string `json:"roomId"`
		SubjectUserID string `json:"subjectUserId"`
		Category      string `json:"category"`
		Detail        string `json:"detail"`
		RequestID     string `json:"requestId"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "举报内容格式不正确")
		return
	}
	if !validReportCategory(input.Category) || len([]rune(strings.TrimSpace(input.Detail))) < 2 || len([]rune(input.Detail)) > 1000 || input.RequestID == "" {
		writeProblem(writer, http.StatusBadRequest, "invalid_report", "请选择举报类型并填写 2 至 1000 字的说明")
		return
	}
	user := currentUser(request)
	if input.RoomID != "" {
		if s.rooms.ActiveRoom(user.ID) != input.RoomID {
			writeProblem(writer, http.StatusForbidden, "report_room_forbidden", "只能举报当前所在房间")
			return
		}
		if input.SubjectUserID != "" {
			actor, ok := s.rooms.Room(input.RoomID)
			if !ok {
				writeProblem(writer, http.StatusNotFound, "room_not_found", "房间不存在或已结束")
				return
			}
			snapshot, err := actor.PublicSnapshot(request.Context())
			if err != nil || !containsPlayer(snapshot.Players, input.SubjectUserID) {
				writeProblem(writer, http.StatusBadRequest, "invalid_report_subject", "被举报用户不在当前房间")
				return
			}
		}
	}
	report, duplicate, err := s.ops.CreateReport(request.Context(), operations.ReportInput{
		ReporterID: user.ID, RoomID: input.RoomID, SubjectUserID: input.SubjectUserID,
		Category: input.Category, Detail: strings.TrimSpace(input.Detail), RequestID: input.RequestID, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"report": report, "duplicate": duplicate})
}

func (s *Server) adminUsers(writer http.ResponseWriter, request *http.Request) {
	if !requirePermission(writer, request, "admin:read") {
		return
	}
	users, err := s.ops.ListUsers(request.Context(), request.URL.Query().Get("q"), queryLimit(request))
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	for index := range users {
		users[index].Balance = s.scores.Balance(users[index].ID)
		users[index].ActiveRoomID = s.rooms.ActiveRoom(users[index].ID)
	}
	writeJSON(writer, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) adminSetUserBanned(writer http.ResponseWriter, request *http.Request) {
	if !requirePermission(writer, request, "user:ban") {
		return
	}
	var input struct {
		Banned    bool   `json:"banned"`
		Reason    string `json:"reason"`
		RequestID string `json:"requestId"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "封禁操作格式不正确")
		return
	}
	administrator := currentUser(request)
	userID := chi.URLParam(request, "userID")
	if userID == administrator.ID && input.Banned {
		writeProblem(writer, http.StatusBadRequest, "cannot_ban_self", "管理员不能封禁自己的账号")
		return
	}
	if input.RequestID == "" || strings.TrimSpace(input.Reason) == "" {
		writeProblem(writer, http.StatusBadRequest, "reason_required", "必须填写原因并提供 requestId")
		return
	}
	user, duplicate, err := s.ops.SetUserBanned(request.Context(), administrator.ID, userID, input.Banned, strings.TrimSpace(input.Reason), input.RequestID, time.Now().UTC())
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	if input.Banned {
		s.realtime.disconnectUser(userID, websocket.StatusPolicyViolation, "account session revoked")
	}
	if err := s.auth.SetBanned(request.Context(), userID, input.Banned); err != nil && !errors.Is(err, auth.ErrUserNotFound) {
		writeProblem(writer, http.StatusServiceUnavailable, "identity_store_unavailable", "用户状态暂时无法同步")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"user": user, "duplicate": duplicate})
}

func (s *Server) adminRooms(writer http.ResponseWriter, request *http.Request) {
	if !requirePermission(writer, request, "admin:read") {
		return
	}
	rooms := s.rooms.AdminRooms(request.Context())
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].CreatedAt.After(rooms[j].CreatedAt) })
	writeJSON(writer, http.StatusOK, map[string]any{"rooms": rooms})
}

func (s *Server) adminRoom(writer http.ResponseWriter, request *http.Request) {
	if !requirePermission(writer, request, "admin:read") {
		return
	}
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
	writeJSON(writer, http.StatusOK, snapshot)
}

func (s *Server) adminReports(writer http.ResponseWriter, request *http.Request) {
	if !requirePermission(writer, request, "admin:read") {
		return
	}
	reports, err := s.ops.ListReports(request.Context(), request.URL.Query().Get("status"), queryLimit(request))
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"reports": reports})
}

func (s *Server) adminHandleReport(writer http.ResponseWriter, request *http.Request) {
	if !requirePermission(writer, request, "report:manage") {
		return
	}
	var input struct {
		Status    string `json:"status"`
		Reason    string `json:"reason"`
		RequestID string `json:"requestId"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "举报处理格式不正确")
		return
	}
	if input.RequestID == "" || strings.TrimSpace(input.Reason) == "" {
		writeProblem(writer, http.StatusBadRequest, "reason_required", "必须填写处理原因并提供 requestId")
		return
	}
	report, duplicate, err := s.ops.HandleReport(
		request.Context(), currentUser(request).ID, chi.URLParam(request, "reportID"), input.Status,
		strings.TrimSpace(input.Reason), input.RequestID, time.Now().UTC(),
	)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"report": report, "duplicate": duplicate})
}

func (s *Server) adminAudits(writer http.ResponseWriter, request *http.Request) {
	if !requirePermission(writer, request, "admin:read") {
		return
	}
	audits, err := s.ops.ListAudits(request.Context(), queryLimit(request))
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"audits": audits})
}

func requirePermission(writer http.ResponseWriter, request *http.Request, permission string) bool {
	if currentUser(request).Has(permission) {
		return true
	}
	writeProblem(writer, http.StatusForbidden, "missing_permission", "缺少权限 "+permission)
	return false
}

func queryLimit(request *http.Request) int {
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	if limit < 1 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func validReportCategory(category string) bool {
	return category == "conduct" || category == "voice" || category == "technical" || category == "other"
}

func containsPlayer(players []room.PlayerSnapshot, userID string) bool {
	for _, player := range players {
		if player.ID == userID {
			return true
		}
	}
	return false
}
