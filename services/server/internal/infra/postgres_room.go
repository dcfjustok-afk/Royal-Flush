package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/royal-flush/royal-flush/services/server/internal/room"
)

func (p *Postgres) CreateRoom(ctx context.Context, record room.Record, ownerSeat room.SeatRecord) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	config, err := json.Marshal(record.Config)
	if err != nil {
		return fmt.Errorf("encode room config: %w", err)
	}
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin room creation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := ensureUserTx(ctx, tx, record.OwnerID, record.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO rooms (id, code, owner_id, config, status, version, created_at)
		VALUES ($1, $2, $3, $4, 'waiting', $5, $6)`,
		record.ID, record.Code, record.OwnerID, config, record.Version, record.CreatedAt); err != nil {
		return fmt.Errorf("create room: %w", err)
	}
	if err := insertSeat(ctx, tx, ownerSeat); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit room creation: %w", err)
	}
	return nil
}

func (p *Postgres) CreateRoomAndSaveState(ctx context.Context, record room.Record, ownerSeat room.SeatRecord, state room.PersistentState) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	config, err := json.Marshal(record.Config)
	if err != nil {
		return fmt.Errorf("encode room config: %w", err)
	}
	rawState, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode room state: %w", err)
	}
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin room creation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := ensureUserTx(ctx, tx, record.OwnerID, record.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO rooms (id, code, owner_id, config, status, version, created_at)
		VALUES ($1, $2, $3, $4, 'waiting', $5, $6)`,
		record.ID, record.Code, record.OwnerID, config, record.Version, record.CreatedAt); err != nil {
		return fmt.Errorf("create room: %w", err)
	}
	if err := insertSeat(ctx, tx, ownerSeat); err != nil {
		return err
	}
	if err := saveRoomStateTx(ctx, tx, state, rawState); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit room creation: %w", err)
	}
	return nil
}

func (p *Postgres) OpenSeat(ctx context.Context, seat room.SeatRecord, claimOwnership bool) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin seat creation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := ensureUserTx(ctx, tx, seat.UserID, seat.JoinedAt); err != nil {
		return err
	}
	if err := insertSeat(ctx, tx, seat); err != nil {
		return err
	}
	if claimOwnership {
		command, err := tx.Exec(ctx, `
			UPDATE rooms SET owner_id = $1, status = 'waiting', empty_since = NULL
			WHERE id = $2 AND status = 'empty' AND ended_at IS NULL`, seat.UserID, seat.RoomID)
		if err != nil {
			return fmt.Errorf("claim empty room ownership: %w", err)
		}
		if command.RowsAffected() != 1 {
			return errors.New("claim empty room ownership: room is not empty")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit seat creation: %w", err)
	}
	return nil
}

func (p *Postgres) AddSeatAllocation(ctx context.Context, seatSessionID string, amount int64) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	command, err := p.pool.Exec(ctx, `
		UPDATE seat_sessions SET allocated_points = allocated_points + $1
		WHERE id = $2 AND left_at IS NULL`, amount, seatSessionID)
	if err != nil {
		return fmt.Errorf("add seat allocation: %w", err)
	}
	if command.RowsAffected() != 1 {
		return errors.New("add seat allocation: active seat session not found")
	}
	return nil
}

func (p *Postgres) UpdateRoomCode(ctx context.Context, roomID, oldCode, newCode string) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	command, err := p.pool.Exec(ctx, `UPDATE rooms SET code = $1 WHERE id = $2 AND code = $3 AND ended_at IS NULL`, newCode, roomID, oldCode)
	if err != nil {
		return fmt.Errorf("update room invite code: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("update room invite code: room is missing or stale")
	}
	return nil
}

func (p *Postgres) UpdateRoomOwner(ctx context.Context, roomID, ownerID string) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	if ownerID == "" {
		_, err := p.pool.Exec(ctx, `UPDATE rooms SET status = 'empty', empty_since = now() WHERE id = $1 AND ended_at IS NULL`, roomID)
		if err != nil {
			return fmt.Errorf("mark room empty: %w", err)
		}
		return nil
	}
	if _, err := p.EnsureUser(ownerID, time.Now().UTC()); err != nil {
		return err
	}
	if _, err := p.pool.Exec(ctx, `
		UPDATE rooms SET owner_id = $1, status = CASE WHEN status = 'empty' THEN 'waiting' ELSE status END, empty_since = NULL
		WHERE id = $2 AND ended_at IS NULL`, ownerID, roomID); err != nil {
		return fmt.Errorf("update room owner: %w", err)
	}
	return nil
}

func (p *Postgres) EndRoom(ctx context.Context, roomID string, endedAt time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	if _, err := p.pool.Exec(ctx, `UPDATE rooms SET status = 'ended', ended_at = COALESCE(ended_at, $2) WHERE id = $1`, roomID, endedAt); err != nil {
		return fmt.Errorf("end room: %w", err)
	}
	return nil
}

func insertSeat(ctx context.Context, tx pgx.Tx, seat room.SeatRecord) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO seat_sessions (id, room_id, user_id, seat, allocated_points, joined_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		seat.ID, seat.RoomID, seat.UserID, seat.Seat, seat.AllocatedPoints, seat.JoinedAt); err != nil {
		return fmt.Errorf("create seat session: %w", err)
	}
	return nil
}
