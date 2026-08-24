package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/room"
)

func (p *Postgres) CreateRoom(ctx context.Context, record room.Record) error {
	if _, err := p.EnsureUser(record.OwnerID, record.CreatedAt); err != nil {
		return err
	}
	config, err := json.Marshal(record.Config)
	if err != nil {
		return fmt.Errorf("encode room config: %w", err)
	}
	if _, err := p.pool.Exec(ctx, `
		INSERT INTO rooms (id, code, owner_id, config, status, version, created_at)
		VALUES ($1, $2, $3, $4, 'waiting', $5, $6)`,
		record.ID, record.Code, record.OwnerID, config, record.Version, record.CreatedAt); err != nil {
		return fmt.Errorf("create room: %w", err)
	}
	return nil
}

func (p *Postgres) UpdateRoomCode(ctx context.Context, roomID, oldCode, newCode string) error {
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
	if _, err := p.pool.Exec(ctx, `UPDATE rooms SET status = 'ended', ended_at = COALESCE(ended_at, $2) WHERE id = $1`, roomID, endedAt); err != nil {
		return fmt.Errorf("end room: %w", err)
	}
	return nil
}
