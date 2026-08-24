package auth

import (
	"errors"
	"testing"
	"time"
)

func TestDevelopmentOTPFlow(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	service := NewService(true, func() time.Time { return now })
	if _, _, err := service.RequestCode("123"); !errors.Is(err, ErrInvalidPhone) {
		t.Fatalf("expected invalid phone, got %v", err)
	}
	code, expiresAt, err := service.RequestCode("13800138000")
	if err != nil || code != "123456" || !expiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("unexpected challenge: %s %s %v", code, expiresAt, err)
	}
	if _, _, err := service.Verify("13800138000", "000000", "小北"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected invalid code, got %v", err)
	}
	user, token, err := service.Verify("13800138000", code, "小北")
	if err != nil || token == "" || user.Nickname != "小北" {
		t.Fatalf("verification failed: %#v %q %v", user, token, err)
	}
	fromSession, ok := service.UserBySession(token)
	if !ok || fromSession.ID != user.ID {
		t.Fatal("session lookup failed")
	}
}

func TestExpiredOTP(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	service := NewService(true, func() time.Time { return now })
	code, _, _ := service.RequestCode("13800138000")
	now = now.Add(6 * time.Minute)
	if _, _, err := service.Verify("13800138000", code, ""); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected expiry, got %v", err)
	}
}

func TestPasswordLogin(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	service := NewService(false, func() time.Time { return now })
	if _, _, err := service.PasswordLogin("19970606473", "wrong", "19970606473", "123456"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	user, token, err := service.PasswordLogin("19970606473", "123456", "19970606473", "123456")
	if err != nil || token == "" || user.ID != "admin-19970606473" || !user.Has("score:reset-all") {
		t.Fatalf("password login failed: %#v %q %v", user, token, err)
	}
	fromSession, ok := service.UserBySession(token)
	if !ok || fromSession.ID != user.ID {
		t.Fatal("password session lookup failed")
	}
}
