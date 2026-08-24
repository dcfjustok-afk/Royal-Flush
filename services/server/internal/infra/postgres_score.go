package infra

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/royal-flush/royal-flush/services/server/internal/idgen"
	"github.com/royal-flush/royal-flush/services/server/internal/score"
)

const postgresOperationTimeout = 5 * time.Second

func (p *Postgres) EnsureUser(userID string, now time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	if _, err := p.pool.Exec(ctx, `
		INSERT INTO users (id, nickname, created_at, updated_at)
		VALUES ($1, $1, $2, $2)
		ON CONFLICT (id) DO NOTHING`, userID, now); err != nil {
		return 0, fmt.Errorf("ensure score user: %w", err)
	}
	return p.balance(ctx, userID)
}

func (p *Postgres) Balance(userID string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	return p.balance(ctx, userID)
}

func (p *Postgres) balance(ctx context.Context, userID string) (int64, error) {
	var balance int64
	if err := p.pool.QueryRow(ctx, `SELECT account_score($1)`, userID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("read score balance: %w", err)
	}
	return balance, nil
}

func (p *Postgres) Add(userID, roomID, requestID string, amount int64, now time.Time) (score.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	tx, err := p.beginScoreMutation(ctx, userID, now)
	if err != nil {
		return score.Result{}, err
	}
	defer tx.Rollback(ctx)

	if result, found, err := findLedgerResult(ctx, tx, userID, requestID); err != nil {
		return score.Result{}, err
	} else if found {
		return result, tx.Commit(ctx)
	}
	var lastAddition time.Time
	err = tx.QueryRow(ctx, `
		SELECT created_at FROM score_addition_requests
		WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`, userID).Scan(&lastAddition)
	if err == nil && now.Sub(lastAddition) < 5*time.Second {
		return score.Result{}, score.ErrRateLimited
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return score.Result{}, fmt.Errorf("read score addition rate limit: %w", err)
	}
	result, err := insertLedgerEntry(ctx, tx, userID, roomID, requestID, score.EntrySelfAdd, amount, "自行增加积分", now)
	if err != nil {
		return score.Result{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO score_addition_requests (user_id, request_id, ledger_id, amount, created_at)
		VALUES ($1, $2, $3, $4, $5)`, userID, requestID, result.Entry.ID, amount, now); err != nil {
		return score.Result{}, fmt.Errorf("record score addition request: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return score.Result{}, fmt.Errorf("commit score addition: %w", err)
	}
	return result, nil
}

func (p *Postgres) ApplySettlement(userID, roomID, seatSessionID string, net int64, now time.Time) (score.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	tx, err := p.beginScoreMutation(ctx, userID, now)
	if err != nil {
		return score.Result{}, err
	}
	defer tx.Rollback(ctx)

	if result, found, err := findSettlementResult(ctx, tx, seatSessionID); err != nil {
		return score.Result{}, err
	} else if found {
		return result, tx.Commit(ctx)
	}
	result, err := insertLedgerEntry(ctx, tx, userID, roomID, seatSessionID, score.EntrySettlement, net, "牌局净输赢结算", now)
	if err != nil {
		return score.Result{}, err
	}
	var remaining int64
	err = tx.QueryRow(ctx, `
		UPDATE seat_sessions
		SET remaining_points = allocated_points + $1, left_at = $2
		WHERE id = $3 AND room_id = $4 AND user_id = $5 AND left_at IS NULL
		RETURNING remaining_points`, net, now, seatSessionID, roomID, userID).Scan(&remaining)
	if err != nil {
		return score.Result{}, fmt.Errorf("close seat session: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO seat_settlements (seat_session_id, ledger_id, net_points, created_at)
		VALUES ($1, $2, $3, $4)`, seatSessionID, result.Entry.ID, net, now); err != nil {
		return score.Result{}, fmt.Errorf("record seat settlement: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return score.Result{}, fmt.Errorf("commit score settlement: %w", err)
	}
	return result, nil
}

func (p *Postgres) ResetAllWithRequest(administrator, reason, requestID string, now time.Time) (score.Epoch, bool, error) {
	if administrator == "" {
		return score.Epoch{}, false, errors.New("administrator is required")
	}
	if reason == "" {
		return score.Epoch{}, false, errors.New("reset reason is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return score.Epoch{}, false, fmt.Errorf("begin score reset: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := ensureUserTx(ctx, tx, administrator, now); err != nil {
		return score.Epoch{}, false, err
	}

	var epoch score.Epoch
	err = scanEpoch(tx.QueryRow(ctx, `
		SELECT id, base_score, COALESCE(administrator_id, 'system'), reason, affected_users, created_at
		FROM score_epochs WHERE request_id = $1`, requestID), &epoch)
	if err == nil {
		return epoch, true, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return score.Epoch{}, false, fmt.Errorf("read score reset request: %w", err)
	}
	err = scanEpoch(tx.QueryRow(ctx, `
		INSERT INTO score_epochs (base_score, administrator_id, reason, request_id, affected_users, created_at)
		VALUES ($1, $2, $3, $4, (SELECT COUNT(*) FROM users), $5)
		RETURNING id, base_score, administrator_id, reason, affected_users, created_at`,
		score.InitialBalance, administrator, reason, requestID, now), &epoch)
	if err != nil {
		return score.Epoch{}, false, fmt.Errorf("create score epoch: %w", err)
	}
	auditID, err := idgen.ID("audit")
	if err != nil {
		return score.Epoch{}, false, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_audit_log
			(id, administrator_id, action, target_type, target_id, reason, request_id, metadata, created_at)
		VALUES ($1, $2, 'score.reset_all', 'score_epoch', $3, $4, $5, jsonb_build_object('affectedUsers', $6), $7)`,
		auditID, administrator, fmt.Sprint(epoch.ID), reason, requestID, epoch.AffectedUsers, now); err != nil {
		return score.Epoch{}, false, fmt.Errorf("record score reset audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return score.Epoch{}, false, fmt.Errorf("commit score reset: %w", err)
	}
	return epoch, false, nil
}

func (p *Postgres) Ledger(userID string) (int64, []score.LedgerEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	if _, err := p.EnsureUser(userID, time.Now().UTC()); err != nil {
		return 0, nil, err
	}
	balance, err := p.balance(ctx, userID)
	if err != nil {
		return 0, nil, err
	}
	rows, err := p.pool.Query(ctx, `
		SELECT id, entry_type, amount, balance_after, COALESCE(room_id, ''), note, created_at, epoch_id, COALESCE(request_id, '')
		FROM score_ledger WHERE user_id = $1 ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return 0, nil, fmt.Errorf("query score ledger: %w", err)
	}
	defer rows.Close()
	entries := make([]score.LedgerEntry, 0)
	for rows.Next() {
		var entry score.LedgerEntry
		if err := rows.Scan(&entry.ID, &entry.Type, &entry.Amount, &entry.Balance, &entry.RoomID, &entry.Note, &entry.CreatedAt, &entry.EpochID, &entry.RequestID); err != nil {
			return 0, nil, fmt.Errorf("scan score ledger: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("iterate score ledger: %w", err)
	}
	var userCreatedAt time.Time
	if err := p.pool.QueryRow(ctx, `SELECT created_at FROM users WHERE id = $1`, userID).Scan(&userCreatedAt); err != nil {
		return 0, nil, fmt.Errorf("read score user: %w", err)
	}
	epochs, err := p.epochs(ctx)
	if err != nil {
		return 0, nil, err
	}
	var initialEpoch score.Epoch
	if err := scanEpoch(p.pool.QueryRow(ctx, `
		SELECT id, base_score, COALESCE(administrator_id, 'system'), reason, affected_users, created_at
		FROM score_epochs WHERE created_at <= $1 ORDER BY id DESC LIMIT 1`, userCreatedAt), &initialEpoch); err != nil {
		return 0, nil, fmt.Errorf("read user's initial score epoch: %w", err)
	}
	if initialEpoch.ID != 0 {
		entries = append(entries, score.LedgerEntry{
			ID: "initial-" + userID, Type: score.EntryInitial, Amount: initialEpoch.BaseScore, Balance: initialEpoch.BaseScore,
			Note: "初始娱乐积分", CreatedAt: userCreatedAt, EpochID: initialEpoch.ID,
		})
	}
	for _, epoch := range epochs {
		if epoch.Administrator == "system" || !epoch.CreatedAt.After(userCreatedAt) {
			continue
		}
		entries = append(entries, score.LedgerEntry{
			ID: fmt.Sprintf("epoch-%d-%s", epoch.ID, userID), Type: score.EntryReset,
			Amount: epoch.BaseScore, Balance: epoch.BaseScore, Note: "平台管理员重置积分周期：" + epoch.Reason,
			CreatedAt: epoch.CreatedAt, EpochID: epoch.ID,
		})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].CreatedAt.After(entries[j].CreatedAt) })
	return balance, entries, nil
}

func (p *Postgres) Epochs() ([]score.Epoch, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	return p.epochs(ctx)
}

func (p *Postgres) epochs(ctx context.Context) ([]score.Epoch, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, base_score, COALESCE(administrator_id, 'system'), reason, affected_users, created_at
		FROM score_epochs ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query score epochs: %w", err)
	}
	defer rows.Close()
	epochs := make([]score.Epoch, 0)
	for rows.Next() {
		var epoch score.Epoch
		if err := rows.Scan(&epoch.ID, &epoch.BaseScore, &epoch.Administrator, &epoch.Reason, &epoch.AffectedUsers, &epoch.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan score epoch: %w", err)
		}
		epochs = append(epochs, epoch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate score epochs: %w", err)
	}
	return epochs, nil
}

func (p *Postgres) beginScoreMutation(ctx context.Context, userID string, now time.Time) (pgx.Tx, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin score mutation: %w", err)
	}
	if err := ensureUserTx(ctx, tx, userID, now); err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, userID); err != nil {
		tx.Rollback(ctx)
		return nil, fmt.Errorf("lock score account: %w", err)
	}
	return tx, nil
}

func ensureUserTx(ctx context.Context, tx pgx.Tx, userID string, now time.Time) error {
	if userID == "" {
		return errors.New("userId is required")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, nickname, created_at, updated_at)
		VALUES ($1, $1, $2, $2)
		ON CONFLICT (id) DO NOTHING`, userID, now); err != nil {
		return fmt.Errorf("ensure score user: %w", err)
	}
	return nil
}

func insertLedgerEntry(ctx context.Context, tx pgx.Tx, userID, roomID, requestID string, typ score.EntryType, amount int64, note string, now time.Time) (score.Result, error) {
	var epochID, baseScore, delta int64
	if err := tx.QueryRow(ctx, `SELECT id, base_score FROM score_epochs ORDER BY id DESC LIMIT 1 FOR SHARE`).Scan(&epochID, &baseScore); err != nil {
		return score.Result{}, fmt.Errorf("read current score epoch: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(amount), 0) FROM score_ledger WHERE user_id = $1 AND epoch_id = $2`, userID, epochID).Scan(&delta); err != nil {
		return score.Result{}, fmt.Errorf("read score delta: %w", err)
	}
	entryID, err := idgen.ID("score")
	if err != nil {
		return score.Result{}, err
	}
	entry := score.LedgerEntry{
		ID: entryID, Type: typ, Amount: amount, Balance: baseScore + delta + amount, RoomID: roomID,
		Note: note, CreatedAt: now, EpochID: epochID, RequestID: requestID,
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO score_ledger (id, user_id, epoch_id, entry_type, amount, balance_after, room_id, request_id, note, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9, $10)`,
		entry.ID, userID, entry.EpochID, entry.Type, entry.Amount, entry.Balance, entry.RoomID, entry.RequestID, entry.Note, entry.CreatedAt); err != nil {
		return score.Result{}, fmt.Errorf("insert score ledger entry: %w", err)
	}
	return score.Result{Balance: entry.Balance, Entry: entry}, nil
}

func findLedgerResult(ctx context.Context, tx pgx.Tx, userID, requestID string) (score.Result, bool, error) {
	row := tx.QueryRow(ctx, `
		SELECT id, entry_type, amount, balance_after, COALESCE(room_id, ''), note, created_at, epoch_id, COALESCE(request_id, '')
		FROM score_ledger WHERE user_id = $1 AND request_id = $2`, userID, requestID)
	return scanLedgerResult(row)
}

func findSettlementResult(ctx context.Context, tx pgx.Tx, seatSessionID string) (score.Result, bool, error) {
	row := tx.QueryRow(ctx, `
		SELECT id, entry_type, amount, balance_after, COALESCE(room_id, ''), note, created_at, epoch_id, COALESCE(request_id, '')
		FROM score_ledger WHERE request_id = $1 AND entry_type = 'game_settlement'`, seatSessionID)
	return scanLedgerResult(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanLedgerResult(row rowScanner) (score.Result, bool, error) {
	var entry score.LedgerEntry
	err := row.Scan(&entry.ID, &entry.Type, &entry.Amount, &entry.Balance, &entry.RoomID, &entry.Note, &entry.CreatedAt, &entry.EpochID, &entry.RequestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return score.Result{}, false, nil
	}
	if err != nil {
		return score.Result{}, false, fmt.Errorf("read score request result: %w", err)
	}
	return score.Result{Balance: entry.Balance, Entry: entry}, true, nil
}

func scanEpoch(row rowScanner, epoch *score.Epoch) error {
	return row.Scan(&epoch.ID, &epoch.BaseScore, &epoch.Administrator, &epoch.Reason, &epoch.AffectedUsers, &epoch.CreatedAt)
}
