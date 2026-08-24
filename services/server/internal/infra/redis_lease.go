package infra

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrLeaseNotOwned = errors.New("room lease is not owned by this instance")

type RoomLease struct {
	client *redis.Client
	prefix string
}

func NewRoomLease(client *redis.Client, prefix string) *RoomLease {
	if prefix == "" {
		prefix = "royal-flush:room-lease:"
	}
	return &RoomLease{client: client, prefix: prefix}
}

func OpenRedis(redisURL string) (*redis.Client, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return redis.NewClient(options), nil
}

func (l *RoomLease) Acquire(ctx context.Context, roomID, owner string, ttl time.Duration) (bool, error) {
	return l.client.SetNX(ctx, l.prefix+roomID, owner, ttl).Result()
}

var renewLease = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0`)

func (l *RoomLease) Renew(ctx context.Context, roomID, owner string, ttl time.Duration) error {
	result, err := renewLease.Run(ctx, l.client, []string{l.prefix + roomID}, owner, ttl.Milliseconds()).Int64()
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrLeaseNotOwned
	}
	return nil
}

var releaseLease = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0`)

func (l *RoomLease) Release(ctx context.Context, roomID, owner string) error {
	result, err := releaseLease.Run(ctx, l.client, []string{l.prefix + roomID}, owner).Int64()
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrLeaseNotOwned
	}
	return nil
}
