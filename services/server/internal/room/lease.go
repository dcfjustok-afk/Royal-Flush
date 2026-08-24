package room

import (
	"context"
	"errors"
	"time"
)

var ErrLeaseNotOwned = errors.New("room lease is not owned by this instance")

type Lease interface {
	Acquire(ctx context.Context, roomID, owner string, ttl time.Duration) (bool, error)
	Renew(ctx context.Context, roomID, owner string, ttl time.Duration) error
	Release(ctx context.Context, roomID, owner string) error
}
