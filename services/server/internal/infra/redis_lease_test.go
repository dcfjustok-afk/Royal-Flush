package infra

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRoomLeaseOwnership(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	lease := NewRoomLease(client, "test:")
	ctx := context.Background()
	acquired, err := lease.Acquire(ctx, "room-1", "instance-a", 10*time.Second)
	if err != nil || !acquired {
		t.Fatalf("acquire failed: %v %v", acquired, err)
	}
	acquired, err = lease.Acquire(ctx, "room-1", "instance-b", 10*time.Second)
	if err != nil || acquired {
		t.Fatalf("second owner acquired lease: %v %v", acquired, err)
	}
	if err := lease.Renew(ctx, "room-1", "instance-b", 20*time.Second); !errors.Is(err, ErrLeaseNotOwned) {
		t.Fatalf("foreign renew should fail: %v", err)
	}
	if err := lease.Renew(ctx, "room-1", "instance-a", 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := lease.Release(ctx, "room-1", "instance-b"); !errors.Is(err, ErrLeaseNotOwned) {
		t.Fatalf("foreign release should fail: %v", err)
	}
	if err := lease.Release(ctx, "room-1", "instance-a"); err != nil {
		t.Fatal(err)
	}
}
