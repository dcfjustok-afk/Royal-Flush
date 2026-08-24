package room

import (
	"context"
	"time"
)

type Record struct {
	ID        string
	Code      string
	OwnerID   string
	Config    Config
	Version   int64
	CreatedAt time.Time
}

type Store interface {
	CreateRoom(ctx context.Context, room Record) error
	UpdateRoomCode(ctx context.Context, roomID, oldCode, newCode string) error
	UpdateRoomOwner(ctx context.Context, roomID, ownerID string) error
	EndRoom(ctx context.Context, roomID string, endedAt time.Time) error
}
