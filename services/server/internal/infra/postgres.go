package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/royal-flush/royal-flush/services/server/internal/idgen"
	"github.com/royal-flush/royal-flush/services/server/internal/room"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func OpenPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Close() {
	p.pool.Close()
}

func (p *Postgres) AppendRoomEvent(ctx context.Context, actorUserID string, event room.Envelope) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	commandTag, err := p.pool.Exec(ctx, `
		INSERT INTO room_events (room_id, version, event_type, request_id, actor_user_id, payload, created_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7)
		ON CONFLICT DO NOTHING`, event.RoomID, event.Version, event.Type, event.RequestID, actorUserID, payload, event.SentAt)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("room event version or request already exists")
	}
	return nil
}

type ScoreMutation struct {
	UserID    string
	Type      string
	Amount    int64
	RoomID    string
	RequestID string
	Note      string
}

type ScoreMutationResult struct {
	LedgerID  string
	EpochID   int64
	Balance   int64
	Duplicate bool
}

func (p *Postgres) ApplyScoreMutation(ctx context.Context, mutation ScoreMutation) (ScoreMutationResult, error) {
	if mutation.UserID == "" || mutation.RequestID == "" {
		return ScoreMutationResult{}, errors.New("userId and requestId are required")
	}
	transaction, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return ScoreMutationResult{}, err
	}
	defer transaction.Rollback(ctx)
	var existing ScoreMutationResult
	err = transaction.QueryRow(ctx, `
		SELECT id, epoch_id, balance_after FROM score_ledger
		WHERE user_id = $1 AND request_id = $2`, mutation.UserID, mutation.RequestID).
		Scan(&existing.LedgerID, &existing.EpochID, &existing.Balance)
	if err == nil {
		existing.Duplicate = true
		return existing, transaction.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ScoreMutationResult{}, err
	}
	var epochID, baseScore int64
	if err := transaction.QueryRow(ctx, `SELECT id, base_score FROM score_epochs ORDER BY id DESC LIMIT 1 FOR SHARE`).Scan(&epochID, &baseScore); err != nil {
		return ScoreMutationResult{}, err
	}
	var delta int64
	if err := transaction.QueryRow(ctx, `SELECT COALESCE(SUM(amount), 0) FROM score_ledger WHERE user_id = $1 AND epoch_id = $2`, mutation.UserID, epochID).Scan(&delta); err != nil {
		return ScoreMutationResult{}, err
	}
	ledgerID, err := idgen.ID("score")
	if err != nil {
		return ScoreMutationResult{}, err
	}
	balance := baseScore + delta + mutation.Amount
	_, err = transaction.Exec(ctx, `
		INSERT INTO score_ledger (id, user_id, epoch_id, entry_type, amount, balance_after, room_id, request_id, note, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9, $10)`,
		ledgerID, mutation.UserID, epochID, mutation.Type, mutation.Amount, balance, mutation.RoomID, mutation.RequestID, mutation.Note, time.Now().UTC())
	if err != nil {
		return ScoreMutationResult{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return ScoreMutationResult{}, err
	}
	return ScoreMutationResult{LedgerID: ledgerID, EpochID: epochID, Balance: balance}, nil
}

func (p *Postgres) CreateScoreEpoch(ctx context.Context, administratorID, reason, requestID string) (int64, bool, error) {
	if administratorID == "" || reason == "" || requestID == "" {
		return 0, false, errors.New("administrator, reason, and requestId are required")
	}
	var epochID int64
	err := p.pool.QueryRow(ctx, `
		INSERT INTO score_epochs (base_score, administrator_id, reason, request_id, affected_users)
		VALUES (1000, $1, $2, $3, (SELECT COUNT(*) FROM users))
		ON CONFLICT (request_id) DO NOTHING RETURNING id`, administratorID, reason, requestID).Scan(&epochID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := p.pool.QueryRow(ctx, `SELECT id FROM score_epochs WHERE request_id = $1`, requestID).Scan(&epochID); err != nil {
			return 0, false, err
		}
		return epochID, true, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("create score epoch: %w", err)
	}
	return epochID, false, nil
}
