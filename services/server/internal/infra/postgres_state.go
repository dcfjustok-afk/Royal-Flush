package infra

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/royal-flush/royal-flush/services/server/internal/poker"
	"github.com/royal-flush/royal-flush/services/server/internal/room"
)

func (p *Postgres) SaveRoomState(ctx context.Context, state room.PersistentState) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode room state: %w", err)
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin room state save: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := saveRoomStateTx(ctx, tx, state, raw); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit room state: %w", err)
	}
	return nil
}

func (p *Postgres) AppendRoomEventAndState(ctx context.Context, actorUserID string, event room.Envelope, state room.PersistentState) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode room event payload: %w", err)
	}
	rawState, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode room state: %w", err)
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin room event save: %w", err)
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `
		INSERT INTO room_events (room_id, version, event_type, request_id, actor_user_id, payload, created_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7)
		ON CONFLICT DO NOTHING`, event.RoomID, event.Version, event.Type, event.RequestID, actorUserID, payload, event.SentAt)
	if err != nil {
		return fmt.Errorf("save room event: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("save room event: request already exists")
	}
	if err := saveRoomStateTx(ctx, tx, state, rawState); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit room event state: %w", err)
	}
	return nil
}

func (p *Postgres) OpenSeatAndAppendRoomEventAndState(ctx context.Context, seat room.SeatRecord, claimOwnership bool, actorUserID string, event room.Envelope, state room.PersistentState) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode room event payload: %w", err)
	}
	rawState, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode room state: %w", err)
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin room join: %w", err)
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
			return fmt.Errorf("claim empty room ownership: room is not empty")
		}
	}
	if err := appendRoomEventTx(ctx, tx, actorUserID, event, payload); err != nil {
		return err
	}
	if err := saveRoomStateTx(ctx, tx, state, rawState); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit room join: %w", err)
	}
	return nil
}

func appendRoomEventTx(ctx context.Context, tx pgx.Tx, actorUserID string, event room.Envelope, payload []byte) error {
	command, err := tx.Exec(ctx, `
		INSERT INTO room_events (room_id, version, event_type, request_id, actor_user_id, payload, created_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7)
		ON CONFLICT DO NOTHING`, event.RoomID, event.Version, event.Type, event.RequestID, actorUserID, payload, event.SentAt)
	if err != nil {
		return fmt.Errorf("save room event: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("save room event: request already exists")
	}
	return nil
}

func (p *Postgres) LoadRoomStates(ctx context.Context) ([]room.PersistentState, error) {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, `
		SELECT room_states.state
		FROM room_states
		JOIN rooms ON rooms.id = room_states.room_id
		WHERE rooms.ended_at IS NULL AND rooms.status IN ('waiting', 'playing')
		ORDER BY rooms.created_at`)
	if err != nil {
		return nil, fmt.Errorf("load active room states: %w", err)
	}
	defer rows.Close()
	states := make([]room.PersistentState, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan room state: %w", err)
		}
		var state room.PersistentState
		if err := json.Unmarshal(raw, &state); err != nil {
			return nil, fmt.Errorf("decode room state: %w", err)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate room states: %w", err)
	}
	return states, nil
}

func saveRoomStateTx(ctx context.Context, tx pgx.Tx, state room.PersistentState, raw []byte) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO room_states (room_id, state, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (room_id) DO UPDATE SET state = EXCLUDED.state, updated_at = EXCLUDED.updated_at`,
		state.Room.ID, raw); err != nil {
		return fmt.Errorf("save room state: %w", err)
	}
	status := roomStateStatus(state)
	if _, err := tx.Exec(ctx, `
		UPDATE rooms SET version = $2, status = $3, code = $4,
			owner_id = CASE WHEN $5 = '' THEN owner_id ELSE $5 END,
			empty_since = CASE WHEN $3 = 'empty' THEN COALESCE(empty_since, now()) ELSE NULL END,
			ended_at = CASE WHEN $3 = 'ended' THEN COALESCE(ended_at, now()) ELSE ended_at END
		WHERE id = $1`, state.Room.ID, state.Room.Version, status, state.Room.Code, state.Room.OwnerID); err != nil {
		return fmt.Errorf("update room state metadata: %w", err)
	}
	return nil
}

func roomStateStatus(state room.PersistentState) string {
	if state.Ended {
		return "ended"
	}
	if state.Room.OwnerID == "" || len(state.Identities) == 0 {
		return "empty"
	}
	switch state.Game.Street {
	case poker.StreetPreflop, poker.StreetFlop, poker.StreetTurn, poker.StreetRiver, poker.StreetShowdown:
		return "playing"
	default:
		return "waiting"
	}
}
