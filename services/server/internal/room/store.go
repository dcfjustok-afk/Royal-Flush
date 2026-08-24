package room

import (
	"context"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/poker"
)

type Record struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	OwnerID   string    `json:"ownerId"`
	Config    Config    `json:"config"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
}

type SeatRecord struct {
	ID              string
	RoomID          string
	UserID          string
	Seat            int
	AllocatedPoints int64
	JoinedAt        time.Time
}

type PersistentState struct {
	Room       Record              `json:"room"`
	Game       poker.State         `json:"game"`
	Identities map[string]Identity `json:"identities"`
	JoinOrder  map[string]int64    `json:"joinOrder"`
	NextJoin   int64               `json:"nextJoin"`
	Muted      map[string]bool     `json:"muted"`
	Messages   []SystemMessage     `json:"messages"`
	Processed  map[string]Envelope `json:"processed"`
	Deadline   time.Time           `json:"deadline"`
	Ended      bool                `json:"ended"`
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

type StateStore interface {
	SaveRoomState(ctx context.Context, state PersistentState) error
	LoadRoomStates(ctx context.Context) ([]PersistentState, error)
}

type AtomicStateStore interface {
	AppendRoomEventAndState(ctx context.Context, actorUserID string, event Envelope, state PersistentState) error
}

type AtomicJoinStore interface {
	OpenSeatAndAppendRoomEventAndState(ctx context.Context, seat SeatRecord, claimOwnership bool, actorUserID string, event Envelope, state PersistentState) error
}

type AtomicCreateStateStore interface {
	CreateRoomAndSaveState(ctx context.Context, room Record, ownerSeat SeatRecord, state PersistentState) error
}
