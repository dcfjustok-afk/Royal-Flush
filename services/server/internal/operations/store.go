package operations

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/idgen"
)

var (
	ErrReasonRequired = errors.New("operation reason is required")
	ErrReportNotFound = errors.New("report not found")
	ErrUserNotFound   = errors.New("user not found")
	ErrInvalidStatus  = errors.New("report status must be resolved or dismissed")
)

type UserIdentity struct {
	ID        string
	Phone     string
	Nickname  string
	CreatedAt time.Time
}

type User struct {
	ID           string    `json:"id"`
	Phone        string    `json:"phone"`
	Nickname     string    `json:"nickname"`
	Balance      int64     `json:"balance"`
	ActiveRoomID string    `json:"activeRoomId,omitempty"`
	Banned       bool      `json:"banned"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Report struct {
	ID            string    `json:"id"`
	ReporterID    string    `json:"reporterId"`
	RoomID        string    `json:"roomId,omitempty"`
	SubjectUserID string    `json:"subjectUserId,omitempty"`
	Category      string    `json:"category"`
	Detail        string    `json:"detail"`
	Status        string    `json:"status"`
	HandledBy     string    `json:"handledBy,omitempty"`
	HandledAt     time.Time `json:"handledAt,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Audit struct {
	ID              string         `json:"id"`
	AdministratorID string         `json:"administratorId"`
	Action          string         `json:"action"`
	TargetType      string         `json:"targetType"`
	TargetID        string         `json:"targetId,omitempty"`
	Reason          string         `json:"reason"`
	RequestID       string         `json:"requestId"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"createdAt"`
}

type ReportInput struct {
	ReporterID    string
	RoomID        string
	SubjectUserID string
	Category      string
	Detail        string
	RequestID     string
	CreatedAt     time.Time
}

type Store interface {
	UpsertUser(ctx context.Context, identity UserIdentity) error
	IsBanned(ctx context.Context, userID string) (bool, error)
	ListUsers(ctx context.Context, query string, limit int) ([]User, error)
	SetUserBanned(ctx context.Context, administratorID, userID string, banned bool, reason, requestID string, now time.Time) (User, bool, error)
	CreateReport(ctx context.Context, input ReportInput) (Report, bool, error)
	ListReports(ctx context.Context, status string, limit int) ([]Report, error)
	HandleReport(ctx context.Context, administratorID, reportID, status, reason, requestID string, now time.Time) (Report, bool, error)
	ListAudits(ctx context.Context, limit int) ([]Audit, error)
}

type MemoryStore struct {
	mu             sync.Mutex
	users          map[string]User
	reports        map[string]Report
	reportRequests map[string]string
	audits         []Audit
	auditRequests  map[string]int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users: make(map[string]User), reports: make(map[string]Report), reportRequests: make(map[string]string),
		auditRequests: make(map[string]int),
	}
}

func (s *MemoryStore) UpsertUser(_ context.Context, identity UserIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	user, exists := s.users[identity.ID]
	if !exists {
		user = User{ID: identity.ID, CreatedAt: identity.CreatedAt, Balance: 1000}
		if user.CreatedAt.IsZero() {
			user.CreatedAt = now
		}
	}
	if identity.Phone != "" {
		user.Phone = identity.Phone
	}
	if identity.Nickname != "" {
		user.Nickname = identity.Nickname
	}
	user.UpdatedAt = now
	s.users[user.ID] = user
	return nil
}

func (s *MemoryStore) IsBanned(_ context.Context, userID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.users[userID].Banned, nil
}

func (s *MemoryStore) ListUsers(_ context.Context, query string, limit int) ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]User, 0)
	for _, user := range s.users {
		haystack := strings.ToLower(user.ID + " " + user.Phone + " " + user.Nickname)
		if query == "" || strings.Contains(haystack, query) {
			result = append(result, user)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) SetUserBanned(_ context.Context, administratorID, userID string, banned bool, reason, requestID string, now time.Time) (User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(reason) == "" {
		return User{}, false, ErrReasonRequired
	}
	if _, exists := s.auditRequests[requestID]; exists {
		return s.users[userID], true, nil
	}
	user, exists := s.users[userID]
	if !exists {
		return User{}, false, ErrUserNotFound
	}
	user.Banned = banned
	user.UpdatedAt = now
	s.users[userID] = user
	action := "user.unban"
	if banned {
		action = "user.ban"
	}
	s.appendAuditLocked(administratorID, action, "user", userID, reason, requestID, nil, now)
	return user, false, nil
}

func (s *MemoryStore) CreateReport(_ context.Context, input ReportInput) (Report, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := input.ReporterID + "\x00" + input.RequestID
	if id := s.reportRequests[key]; id != "" {
		return s.reports[id], true, nil
	}
	id, err := idgen.ID("report")
	if err != nil {
		return Report{}, false, err
	}
	report := Report{
		ID: id, ReporterID: input.ReporterID, RoomID: input.RoomID, SubjectUserID: input.SubjectUserID,
		Category: input.Category, Detail: input.Detail, Status: "open", CreatedAt: input.CreatedAt,
	}
	s.reports[id] = report
	s.reportRequests[key] = id
	return report, false, nil
}

func (s *MemoryStore) ListReports(_ context.Context, status string, limit int) ([]Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Report, 0)
	for _, report := range s.reports {
		if status == "" || report.Status == status {
			result = append(result, report)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) HandleReport(_ context.Context, administratorID, reportID, status, reason, requestID string, now time.Time) (Report, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if status != "resolved" && status != "dismissed" {
		return Report{}, false, ErrInvalidStatus
	}
	if strings.TrimSpace(reason) == "" {
		return Report{}, false, ErrReasonRequired
	}
	if _, exists := s.auditRequests[requestID]; exists {
		report, ok := s.reports[reportID]
		if !ok {
			return Report{}, true, ErrReportNotFound
		}
		return report, true, nil
	}
	report, ok := s.reports[reportID]
	if !ok {
		return Report{}, false, ErrReportNotFound
	}
	report.Status, report.HandledBy, report.HandledAt = status, administratorID, now
	s.reports[reportID] = report
	s.appendAuditLocked(administratorID, "report."+status, "report", reportID, reason, requestID, map[string]any{"category": report.Category}, now)
	return report, false, nil
}

func (s *MemoryStore) ListAudits(_ context.Context, limit int) ([]Audit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.audits) {
		limit = len(s.audits)
	}
	return append([]Audit(nil), s.audits[:limit]...), nil
}

func (s *MemoryStore) appendAuditLocked(administratorID, action, targetType, targetID, reason, requestID string, metadata map[string]any, now time.Time) {
	id, _ := idgen.ID("audit")
	audit := Audit{
		ID: id, AdministratorID: administratorID, Action: action, TargetType: targetType,
		TargetID: targetID, Reason: reason, RequestID: requestID, Metadata: metadata, CreatedAt: now,
	}
	s.audits = append([]Audit{audit}, s.audits...)
	s.auditRequests[requestID] = 0
}
