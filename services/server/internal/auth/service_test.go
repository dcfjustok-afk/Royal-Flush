package auth

import (
	"context"
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
	if _, _, err := service.Verify(context.Background(), "13800138000", "000000", "小北"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected invalid code, got %v", err)
	}
	user, token, err := service.Verify(context.Background(), "13800138000", code, "小北")
	if err != nil || token == "" || user.Nickname != "小北" {
		t.Fatalf("verification failed: %#v %q %v", user, token, err)
	}
	fromSession, ok, err := service.UserBySession(context.Background(), token)
	if err != nil || !ok || fromSession.ID != user.ID {
		t.Fatal("session lookup failed")
	}
}

func TestExpiredOTP(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	service := NewService(true, func() time.Time { return now })
	code, _, _ := service.RequestCode("13800138000")
	now = now.Add(6 * time.Minute)
	if _, _, err := service.Verify(context.Background(), "13800138000", code, ""); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected expiry, got %v", err)
	}
}

func TestPasswordLogin(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	service := NewService(false, func() time.Time { return now })
	if _, _, err := service.PasswordLogin(context.Background(), "19970606473", "wrong", "19970606473", "123456"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	user, token, err := service.PasswordLogin(context.Background(), "19970606473", "123456", "19970606473", "123456")
	if err != nil || token == "" || user.ID != "admin-19970606473" || !user.Has("score:reset-all") {
		t.Fatalf("password login failed: %#v %q %v", user, token, err)
	}
	fromSession, ok, err := service.UserBySession(context.Background(), token)
	if err != nil || !ok || fromSession.ID != user.ID {
		t.Fatal("password session lookup failed")
	}
}

func TestRegisteredAccountAndSessionSurviveServiceRestart(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	store := NewMemoryStore()
	service := NewServiceWithStore(false, func() time.Time { return now }, store)
	if _, _, err := service.Register(ctx, "13800138000", "short", "小北"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected weak password, got %v", err)
	}
	user, token, err := service.Register(ctx, "13800138000", "table2026", "小北")
	if err != nil || token == "" || user.Phone != "13800138000" {
		t.Fatalf("register failed: %#v %q %v", user, token, err)
	}
	if _, _, err := service.Register(ctx, "13800138000", "table2026", "另一个人"); !errors.Is(err, ErrPhoneRegistered) {
		t.Fatalf("expected duplicate phone, got %v", err)
	}

	restarted := NewServiceWithStore(false, func() time.Time { return now }, store)
	fromSession, ok, err := restarted.UserBySession(ctx, token)
	if err != nil || !ok || fromSession.ID != user.ID {
		t.Fatalf("persisted session lookup failed: %#v %v %v", fromSession, ok, err)
	}
	loggedIn, nextToken, err := restarted.Login(ctx, "13800138000", "table2026")
	if err != nil || nextToken == "" || loggedIn.ID != user.ID {
		t.Fatalf("login failed: %#v %q %v", loggedIn, nextToken, err)
	}
	if err := restarted.Logout(ctx, nextToken); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := restarted.UserBySession(ctx, nextToken); err != nil || ok {
		t.Fatalf("logged out session remains valid: %v %v", ok, err)
	}
}
