package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sync"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/idgen"
)

var (
	ErrInvalidPhone = errors.New("请输入有效的中国大陆手机号")
	ErrInvalidCode  = errors.New("验证码无效或已过期")
	ErrInvalidCredentials = errors.New("账号或密码错误")
)

var mainlandPhone = regexp.MustCompile(`^1[3-9][0-9]{9}$`)

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
	mu           sync.Mutex
	now          func() time.Time
	development  bool
	challenges   map[string]challenge
	usersByPhone map[string]*User
	usersByID    map[string]*User
	sessions     map[string]string
}

func NewService(development bool, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		now: now, development: development, challenges: make(map[string]challenge),
		usersByPhone: make(map[string]*User), usersByID: make(map[string]*User), sessions: make(map[string]string),
	}
}

func (s *Service) RequestCode(phone string) (string, time.Time, error) {
	if !mainlandPhone.MatchString(phone) {
		return "", time.Time{}, ErrInvalidPhone
	}
	code := "123456"
	if !s.development {
		generated, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return "", time.Time{}, err
		}
		code = fmt.Sprintf("%06d", generated.Int64())
	}
	expiresAt := s.now().UTC().Add(5 * time.Minute)
	s.mu.Lock()
	s.challenges[phone] = challenge{code: code, expiresAt: expiresAt}
	s.mu.Unlock()
	if s.development {
		return code, expiresAt, nil
	}
	return "", expiresAt, nil
}

func (s *Service) Verify(phone, code, nickname string) (User, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	challenge, ok := s.challenges[phone]
	if !ok || s.now().After(challenge.expiresAt) || subtle.ConstantTimeCompare([]byte(code), []byte(challenge.code)) != 1 {
		return User{}, "", ErrInvalidCode
	}
	delete(s.challenges, phone)
	user := s.usersByPhone[phone]
	if user == nil {
		id, err := idgen.ID("user")
		if err != nil {
			return User{}, "", err
		}
		if nickname == "" {
			nickname = "牌友" + phone[len(phone)-4:]
		}
		user = &User{ID: id, Phone: phone, Nickname: nickname, Permissions: make(map[string]bool), CreatedAt: s.now().UTC()}
		s.usersByPhone[phone] = user
		s.usersByID[id] = user
	} else if nickname != "" {
		user.Nickname = nickname
	}
	token, err := idgen.ID("session")
	if err != nil {
		return User{}, "", err
	}
	s.sessions[token] = user.ID
	return cloneUser(user), token, nil
}

func (s *Service) PasswordLogin(account, password, configuredAccount, configuredPassword string) (User, string, error) {
	if configuredAccount == "" || configuredPassword == "" || subtle.ConstantTimeCompare([]byte(account), []byte(configuredAccount)) != 1 || subtle.ConstantTimeCompare([]byte(password), []byte(configuredPassword)) != 1 {
		return User{}, "", ErrInvalidCredentials
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	userID := "admin-" + configuredAccount
	user := s.usersByID[userID]
	if user == nil {
		user = &User{
			ID: userID, Phone: configuredAccount, Nickname: "平台管理员",
			Permissions: map[string]bool{"score:reset-all": true, "admin:read": true, "user:ban": true, "report:manage": true},
			CreatedAt: s.now().UTC(),
		}
		s.usersByID[userID] = user
	} else {
		user.Permissions["score:reset-all"] = true
		user.Permissions["admin:read"] = true
		user.Permissions["user:ban"] = true
		user.Permissions["report:manage"] = true
	}
	token, err := idgen.ID("session")
	if err != nil {
		return User{}, "", err
	}
	s.sessions[token] = user.ID
	return cloneUser(user), token, nil
}

func (s *Service) UserBySession(token string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user := s.usersByID[s.sessions[token]]
	if user == nil || user.Banned {
		return User{}, false
	}
	return cloneUser(user), true
}

func (s *Service) EnsureDevelopmentUser(userID, nickname string, permissions ...string) User {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user := s.usersByID[userID]; user != nil {
		for _, permission := range permissions {
			user.Permissions[permission] = true
		}
		return cloneUser(user)
	}
	if nickname == "" {
		nickname = "本地玩家"
	}
	user := &User{ID: userID, Nickname: nickname, Phone: "", Permissions: make(map[string]bool), CreatedAt: s.now().UTC()}
	for _, permission := range permissions {
		user.Permissions[permission] = true
	}
	s.usersByID[userID] = user
	return cloneUser(user)
}

func (s *Service) SetBanned(userID string, banned bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user := s.usersByID[userID]; user != nil {
		user.Banned = banned
	}
}

func cloneUser(user *User) User {
	copy := *user
	copy.Permissions = make(map[string]bool, len(user.Permissions))
	for permission, enabled := range user.Permissions {
		copy.Permissions[permission] = enabled
	}
	return copy
}
