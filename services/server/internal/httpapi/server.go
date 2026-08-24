package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/royal-flush/royal-flush/services/server/internal/auth"
	"github.com/royal-flush/royal-flush/services/server/internal/operations"
	"github.com/royal-flush/royal-flush/services/server/internal/poker"
	"github.com/royal-flush/royal-flush/services/server/internal/room"
	"github.com/royal-flush/royal-flush/services/server/internal/score"
	"github.com/royal-flush/royal-flush/services/server/internal/voice"
)

type Config struct {
	Development    bool
	AllowedOrigins []string
	Voice          voice.Config
	Readiness      func(context.Context) error
	ScoreStore     score.Store
	RoomStore      room.Store
	RoomLease      room.Lease
	InstanceID     string
	Operations     operations.Store
	AdminUserIDs   map[string]bool
}

type Server struct {
	config Config
	log    *slog.Logger
	auth   *auth.Service
	scores *score.Service
	rooms  *room.Manager
	ops    operations.Store
	router http.Handler
}

type contextKey string

const userContextKey contextKey = "authenticated-user"

func New(config Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	authService := auth.NewService(config.Development, nil)
	scoreService := score.NewServiceWithStore(config.ScoreStore, nil)
	operationsStore := config.Operations
	if operationsStore == nil {
		operationsStore = operations.NewMemoryStore()
	}
	server := &Server{config: config, log: logger, auth: authService, scores: scoreService, ops: operationsStore}
	server.rooms = room.NewManagerWithInfrastructure(scoreService, config.RoomStore, config.RoomLease, config.InstanceID)
	server.router = server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) Close() {
	s.rooms.Close()
}

func (s *Server) Restore(ctx context.Context) error {
	return s.rooms.Restore(ctx)
}

func (s *Server) routes() http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(s.cors)
	router.Get("/api/v1/health", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
	})
	router.Get("/api/v1/ready", s.readiness)
	router.Post("/api/v1/auth/otp/request", s.requestOTP)
	router.Post("/api/v1/auth/otp/verify", s.verifyOTP)
	router.Get("/api/v1/rooms/{roomID}/public", s.publicRoom)
	router.Group(func(protected chi.Router) {
		protected.Use(s.authenticate)
		protected.Get("/api/v1/me", s.me)
		protected.Post("/api/v1/me/score-additions", s.addScore)
		protected.Get("/api/v1/me/score-ledger", s.scoreLedger)
		protected.Post("/api/v1/reports", s.createReport)
		protected.Post("/api/v1/rooms", s.createRoom)
		protected.Post("/api/v1/rooms/{roomID}/join", s.joinRoom)
		protected.Get("/api/v1/rooms/{roomID}/snapshot", s.roomSnapshot)
		protected.Post("/api/v1/rooms/{roomID}/commands", s.roomCommand)
		protected.Post("/api/v1/rooms/{roomID}/voice-token", s.voiceToken)
		protected.Get("/api/v1/rooms/{roomID}/events", s.roomEvents)
		protected.Post("/api/v1/admin/score-resets", s.resetScores)
		protected.Get("/api/v1/admin/score-epochs", s.scoreEpochs)
		protected.Get("/api/v1/admin/users", s.adminUsers)
		protected.Post("/api/v1/admin/users/{userID}/ban-actions", s.adminSetUserBanned)
		protected.Get("/api/v1/admin/rooms", s.adminRooms)
		protected.Get("/api/v1/admin/rooms/{roomID}", s.adminRoom)
		protected.Get("/api/v1/admin/reports", s.adminReports)
		protected.Post("/api/v1/admin/reports/{reportID}/resolution", s.adminHandleReport)
		protected.Get("/api/v1/admin/audit-log", s.adminAudits)
	})
	return router
}

func (s *Server) readiness(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	if s.config.Readiness != nil {
		if err := s.config.Readiness(ctx); err != nil {
			s.log.Warn("readiness check failed", "error", err)
			writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "time": time.Now().UTC()})
			return
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{"status": "ready", "time": time.Now().UTC()})
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var user auth.User
		var ok bool
		if cookie, err := request.Cookie("rf_session"); err == nil {
			user, ok = s.auth.UserBySession(cookie.Value)
		}
		if !ok && s.config.Development {
			userID := request.Header.Get("X-User-ID")
			if userID == "" {
				userID = "demo-user"
			}
			permissions := []string(nil)
			if request.Header.Get("X-Admin") == "true" {
				permissions = append(permissions, "score:reset-all", "admin:read", "user:ban", "report:manage")
			}
			user = s.auth.EnsureDevelopmentUser(userID, request.Header.Get("X-User-Name"), permissions...)
			ok = true
		}
		if !ok {
			writeProblem(writer, http.StatusUnauthorized, "authentication_required", "请先完成手机号验证码登录")
			return
		}
		if s.config.AdminUserIDs[user.ID] {
			for _, permission := range []string{"score:reset-all", "admin:read", "user:ban", "report:manage"} {
				user.Permissions[permission] = true
			}
		}
		if err := s.ops.UpsertUser(request.Context(), operations.UserIdentity{ID: user.ID, Phone: user.Phone, Nickname: user.Nickname, CreatedAt: user.CreatedAt}); err != nil {
			writeProblem(writer, http.StatusServiceUnavailable, "identity_store_unavailable", "用户资料暂时无法保存")
			return
		}
		banned, err := s.ops.IsBanned(request.Context(), user.ID)
		if err != nil {
			writeProblem(writer, http.StatusServiceUnavailable, "identity_store_unavailable", "用户状态暂时无法确认")
			return
		}
		if banned {
			writeProblem(writer, http.StatusForbidden, "account_banned", "账号已被平台管理员封禁")
			return
		}
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), userContextKey, user)))
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if origin != "" && s.originAllowed(origin) {
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Access-Control-Allow-Credentials", "true")
			writer.Header().Set("Vary", "Origin")
		}
		if request.Method == http.MethodOptions {
			writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-ID, X-User-Name, X-Admin")
			writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) originAllowed(origin string) bool {
	if len(s.config.AllowedOrigins) == 0 && s.config.Development {
		return strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:")
	}
	for _, allowed := range s.config.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func currentUser(request *http.Request) auth.User {
	return request.Context().Value(userContextKey).(auth.User)
}

func decodeJSON(request *http.Request, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeProblem(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, map[string]any{"code": code, "message": message})
}

func writeDomainError(writer http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := "invalid_request"
	switch {
	case errors.Is(err, room.ErrForbidden):
		status, code = http.StatusForbidden, "forbidden"
	case errors.Is(err, room.ErrCannotRemoveOwner):
		status, code = http.StatusBadRequest, "cannot_remove_owner"
	case errors.Is(err, room.ErrVersionConflict):
		status, code = http.StatusConflict, "version_conflict"
	case errors.Is(err, room.ErrPlayerNotSeated):
		status, code = http.StatusNotFound, "player_not_seated"
	case errors.Is(err, room.ErrAlreadySeated), errors.Is(err, poker.ErrSeatOccupied), errors.Is(err, poker.ErrPlayerSeated):
		status, code = http.StatusConflict, "seat_conflict"
	case errors.Is(err, score.ErrRateLimited):
		status, code = http.StatusTooManyRequests, "score_rate_limited"
	case errors.Is(err, auth.ErrInvalidCode):
		status, code = http.StatusUnauthorized, "invalid_otp"
	case errors.Is(err, operations.ErrUserNotFound), errors.Is(err, operations.ErrReportNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, operations.ErrReasonRequired), errors.Is(err, operations.ErrInvalidStatus):
		status, code = http.StatusBadRequest, "invalid_operation"
	}
	writeProblem(writer, status, code, err.Error())
}
