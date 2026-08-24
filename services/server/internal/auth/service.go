package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/royal-flush/royal-flush/services/server/internal/idgen"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidPhone       = errors.New("请输入有效的中国大陆手机号")
	ErrInvalidCode        = errors.New("验证码无效或已过期")
	ErrOTPUnavailable     = errors.New("验证码登录仅在开发环境可用")
	ErrInvalidCredentials = errors.New("账号或密码错误")
	ErrPhoneRegistered    = errors.New("该手机号已经注册，请直接登录")
	ErrWeakPassword       = errors.New("密码需为 8 至 72 个字符，并同时包含字母和数字")
	ErrInvalidNickname    = errors.New("昵称需为 1 至 20 个字符")
	ErrAccountBanned      = errors.New("账号已被平台管理员封禁")
)

var (
	mainlandPhone = regexp.MustCompile(`^1[3-9][0-9]{9}$`)
	hasLetter     = regexp.MustCompile(`[A-Za-z]`)
	hasDigit      = regexp.MustCompile(`[0-9]`)
)

const sessionLifetime = 30 * 24 * time.Hour

type User struct {
	ID          string          `json:"id"`
	Phone       string          `json:"phone"`
	Nickname    string          `json:"nickname"`
	Permissions map[string]bool `json:"permissions"`
	Banned      bool            `json:"banned"`
	CreatedAt   time.Time       `json:"createdAt"`
}

func (u User) Has(permission string) bool {
	return u.Permissions[permission]
}

type challenge struct {
	code      string
	expiresAt time.Time
}

type Service struct {
	mu          sync.Mutex
	now         func() time.Time
	development bool
	challenges  map[string]challenge
	store       Store
}

func NewService(development bool, now func() time.Time) *Service {
	return NewServiceWithStore(development, now, NewMemoryStore())
}

func NewServiceWithStore(development bool, now func() time.Time, store Store) *Service {
	if now == nil {
		now = time.Now
	}
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{now: now, development: development, challenges: make(map[string]challenge), store: store}
}

func (s *Service) RequestCode(phone string) (string, time.Time, error) {
	phone = strings.TrimSpace(phone)
	if !mainlandPhone.MatchString(phone) {
		return "", time.Time{}, ErrInvalidPhone
	}
	if !s.development {
		return "", time.Time{}, ErrOTPUnavailable
	}
	code := "123456"
	expiresAt := s.now().UTC().Add(5 * time.Minute)
	s.mu.Lock()
	s.challenges[phone] = challenge{code: code, expiresAt: expiresAt}
	s.mu.Unlock()
	return code, expiresAt, nil
}

func (s *Service) Verify(ctx context.Context, phone, code, nickname string) (User, string, error) {
	phone = strings.TrimSpace(phone)
	s.mu.Lock()
	challenge, ok := s.challenges[phone]
	if !ok || s.now().After(challenge.expiresAt) || subtle.ConstantTimeCompare([]byte(code), []byte(challenge.code)) != 1 {
		s.mu.Unlock()
		return User{}, "", ErrInvalidCode
	}
	delete(s.challenges, phone)
	s.mu.Unlock()

	record, found, err := s.store.UserByPhone(ctx, phone)
	if err != nil {
		return User{}, "", err
	}
	if !found {
		id, err := idgen.ID("user")
		if err != nil {
			return User{}, "", err
		}
		if strings.TrimSpace(nickname) == "" {
			nickname = "牌友" + phone[len(phone)-4:]
		}
		nickname, err = normalizeNickname(nickname)
		if err != nil {
			return User{}, "", err
		}
		record = StoredUser{User: User{ID: id, Phone: phone, Nickname: nickname, Permissions: map[string]bool{}, CreatedAt: s.now().UTC()}}
		if err := s.store.SaveUser(ctx, record); err != nil {
			return User{}, "", err
		}
	} else if strings.TrimSpace(nickname) != "" {
		nickname, err = normalizeNickname(nickname)
		if err != nil {
			return User{}, "", err
		}
		record.Nickname = nickname
		if err := s.store.SaveUser(ctx, record); err != nil {
			return User{}, "", err
		}
	}
	return s.createSession(ctx, record.User)
}

func (s *Service) Register(ctx context.Context, phone, password, nickname string) (User, string, error) {
	phone = strings.TrimSpace(phone)
	if !mainlandPhone.MatchString(phone) {
		return User{}, "", ErrInvalidPhone
	}
	if err := validatePassword(password); err != nil {
		return User{}, "", err
	}
	var err error
	nickname, err = normalizeNickname(nickname)
	if err != nil {
		return User{}, "", err
	}
	if _, found, err := s.store.UserByPhone(ctx, phone); err != nil {
		return User{}, "", err
	} else if found {
		return User{}, "", ErrPhoneRegistered
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, "", err
	}
	id, err := idgen.ID("user")
	if err != nil {
		return User{}, "", err
	}
	user := User{ID: id, Phone: phone, Nickname: nickname, Permissions: map[string]bool{}, CreatedAt: s.now().UTC()}
	if err := s.store.SaveUser(ctx, StoredUser{User: user, PasswordHash: string(hash)}); err != nil {
		return User{}, "", err
	}
	return s.createSession(ctx, user)
}

func (s *Service) Login(ctx context.Context, phone, password string) (User, string, error) {
	phone = strings.TrimSpace(phone)
	if !mainlandPhone.MatchString(phone) {
		return User{}, "", ErrInvalidCredentials
	}
	record, found, err := s.store.UserByPhone(ctx, phone)
	if err != nil {
		return User{}, "", err
	}
	if !found || record.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(password)) != nil {
		return User{}, "", ErrInvalidCredentials
	}
	if record.Banned {
		return User{}, "", ErrAccountBanned
	}
	return s.createSession(ctx, record.User)
}

func (s *Service) PasswordLogin(ctx context.Context, account, password, configuredAccount, configuredPassword string) (User, string, error) {
	if configuredAccount == "" || configuredPassword == "" || subtle.ConstantTimeCompare([]byte(account), []byte(configuredAccount)) != 1 || subtle.ConstantTimeCompare([]byte(password), []byte(configuredPassword)) != 1 {
		return User{}, "", ErrInvalidCredentials
	}
	userID := "admin-" + configuredAccount
	record, found, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return User{}, "", err
	}
	if !found {
		record = StoredUser{User: User{ID: userID, Nickname: "平台管理员", Permissions: map[string]bool{}, CreatedAt: s.now().UTC()}}
	}
	for _, permission := range []string{"score:reset-all", "admin:read", "user:ban", "report:manage"} {
		record.Permissions[permission] = true
	}
	if err := s.store.SaveUser(ctx, record); err != nil {
		return User{}, "", err
	}
	return s.createSession(ctx, record.User)
}

func (s *Service) UserBySession(ctx context.Context, token string) (User, bool, error) {
	if token == "" {
		return User{}, false, nil
	}
	record, ok, err := s.store.UserBySession(ctx, sessionHash(token), s.now().UTC())
	if err != nil || !ok || record.Banned {
		return User{}, false, err
	}
	return cloneUser(&record.User), true, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, sessionHash(token))
}

func (s *Service) UpdateNickname(ctx context.Context, userID, nickname string) (User, error) {
	nickname, err := normalizeNickname(nickname)
	if err != nil {
		return User{}, err
	}
	record, found, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return User{}, err
	}
	if !found {
		return User{}, ErrInvalidCredentials
	}
	record.Nickname = nickname
	if err := s.store.SaveUser(ctx, record); err != nil {
		return User{}, err
	}
	return cloneUser(&record.User), nil
}

func (s *Service) EnsureDevelopmentUser(ctx context.Context, userID, nickname string, permissions ...string) (User, error) {
	record, found, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return User{}, err
	}
	if !found {
		if nickname == "" {
			nickname = "本地玩家"
		}
		record = StoredUser{User: User{ID: userID, Nickname: nickname, Permissions: map[string]bool{}, CreatedAt: s.now().UTC()}}
	}
	for _, permission := range permissions {
		record.Permissions[permission] = true
	}
	if err := s.store.SaveUser(ctx, record); err != nil {
		return User{}, err
	}
	return cloneUser(&record.User), nil
}

func (s *Service) SetBanned(ctx context.Context, userID string, banned bool) error {
	return s.store.SetBanned(ctx, userID, banned)
}

func (s *Service) createSession(ctx context.Context, user User) (User, string, error) {
	token, err := idgen.ID("session")
	if err != nil {
		return User{}, "", err
	}
	if err := s.store.SaveSession(ctx, sessionHash(token), user.ID, s.now().UTC().Add(sessionLifetime)); err != nil {
		return User{}, "", err
	}
	return cloneUser(&user), token, nil
}

func sessionHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func validatePassword(password string) error {
	if len(password) < 8 || len(password) > 72 || !hasLetter.MatchString(password) || !hasDigit.MatchString(password) {
		return ErrWeakPassword
	}
	return nil
}

func normalizeNickname(nickname string) (string, error) {
	nickname = strings.TrimSpace(nickname)
	if count := utf8.RuneCountInString(nickname); count < 1 || count > 20 {
		return "", ErrInvalidNickname
	}
	return nickname, nil
}

func cloneUser(user *User) User {
	copy := *user
	copy.Permissions = make(map[string]bool, len(user.Permissions))
	for permission, enabled := range user.Permissions {
		copy.Permissions[permission] = enabled
	}
	return copy
}
