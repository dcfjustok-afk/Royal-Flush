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

type SeatRecord struct {
	ID              string
	RoomID          string
	UserID          string
	Seat            int
	AllocatedPoints int64
	JoinedAt        time.Time
}

type Store interface {
	CreateRoom(ctx context.Context, room Record, ownerSeat SeatRecord) error
	OpenSeat(ctx context.Context, seat SeatRecord, claimOwnership bool) error
	AddSeatAllocation(ctx context.Context, seatSessionID string, amount int64) error
	UpdateRoomCode(ctx context.Context, roomID, oldCode, newCode string) error
	UpdateRoomOwner(ctx context.Context, roomID, ownerID string) error
	EndRoom(ctx context.Context, roomID string, endedAt time.Time) error
	AppendRoomEvent(ctx context.Context, actorUserID string, event Envelope) error
}
