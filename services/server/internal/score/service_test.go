package score

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestScoreLifecycleAndIdempotency(t *testing.T) {
	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	service := NewService(func() time.Time { return now })
	if got := service.EnsureUser("u1"); got != 1000 {
		t.Fatalf("initial balance = %d", got)
	}
	first, err := service.Add("u1", "room-1", "request-1", 500)
	if err != nil || first.Balance != 1500 {
		t.Fatalf("addition failed: %#v, %v", first, err)
	}
	retry, err := service.Add("u1", "room-1", "request-1", 999)
	if err != nil || retry != first {
		t.Fatalf("idempotent retry changed result: %#v, %v", retry, err)
	}
	if _, err := service.Add("u1", "room-1", "request-2", 1); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected rate limit, got %v", err)
	}
	now = now.Add(5 * time.Second)
	if _, err := service.Add("u1", "room-1", "request-2", 1); err != nil {
		t.Fatal(err)
	}
	settled, err := service.ApplySettlement("u1", "room-1", "seat-session-1", -2000)
	if err != nil || settled.Balance != -499 {
		t.Fatalf("negative balance settlement failed: %#v, %v", settled, err)
	}
	settledRetry, err := service.ApplySettlement("u1", "room-1", "seat-session-1", 5000)
	if err != nil || settledRetry != settled {
		t.Fatalf("settlement was not idempotent: %#v, %v", settledRetry, err)
	}
}

func TestResetCreatesNewEpochAndOldHandSettlesIntoIt(t *testing.T) {
	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	service := NewService(func() time.Time { return now })
	_, _ = service.Add("u1", "room-1", "add-1", 400)
	service.EnsureUser("u2")
	now = now.Add(time.Minute)
	epoch, err := service.ResetAll("admin-1", "新赛季基线")
	if err != nil {
		t.Fatal(err)
	}
	if epoch.ID != 2 || epoch.AffectedUsers != 2 || service.Balance("u1") != 1000 {
		t.Fatalf("unexpected reset result: %#v, balance=%d", epoch, service.Balance("u1"))
	}
	result, err := service.ApplySettlement("u1", "room-1", "old-hand-seat-session", -1400)
	if err != nil || result.Balance != -400 {
		t.Fatalf("old hand did not settle into current epoch: %#v, %v", result, err)
	}
	_, entries := service.Ledger("u1")
	if len(entries) < 3 || entries[0].Type != EntrySettlement {
		t.Fatalf("ledger did not preserve entries: %#v", entries)
	}
}

func TestConcurrentAdditionIdempotency(t *testing.T) {
	service := NewService(func() time.Time { return time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC) })
	var wg sync.WaitGroup
	errorsSeen := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := service.Add("u1", "room-1", "same-request", 7)
			if err != nil {
				errorsSeen <- err
				return
			}
			if result.Balance != 1007 {
				errorsSeen <- errors.New("duplicate application")
			}
		}()
	}
	wg.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Error(err)
	}
	if balance := service.Balance("u1"); balance != 1007 {
		t.Fatalf("balance = %d", balance)
	}
}
