package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/royal-flush/royal-flush/services/server/internal/idgen"
	"github.com/royal-flush/royal-flush/services/server/internal/operations"
)

func (p *Postgres) UpsertUser(ctx context.Context, identity operations.UserIdentity) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	if identity.CreatedAt.IsZero() {
		identity.CreatedAt = time.Now().UTC()
	}
	if identity.Nickname == "" {
		identity.Nickname = identity.ID
	}
	if _, err := p.pool.Exec(ctx, `
		INSERT INTO users (id, phone, nickname, created_at, updated_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, now())
		ON CONFLICT (id) DO UPDATE SET
			phone = COALESCE(NULLIF(EXCLUDED.phone, ''), users.phone),
			nickname = EXCLUDED.nickname,
			updated_at = now()`, identity.ID, identity.Phone, identity.Nickname, identity.CreatedAt); err != nil {
		return fmt.Errorf("upsert operations user: %w", err)
	}
	return nil
}

func (p *Postgres) IsBanned(ctx context.Context, userID string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	var banned bool
	err := p.pool.QueryRow(ctx, `SELECT banned_at IS NOT NULL FROM users WHERE id = $1`, userID).Scan(&banned)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read user ban status: %w", err)
	}
	return banned, nil
}

func (p *Postgres) ListUsers(ctx context.Context, query string, limit int) ([]operations.User, error) {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	limit = clampLimit(limit)
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := p.pool.Query(ctx, `
		SELECT users.id, COALESCE(users.phone, ''), users.nickname, account_score(users.id),
			COALESCE(active.room_id, ''), users.banned_at IS NOT NULL, users.created_at, users.updated_at
		FROM users
		LEFT JOIN LATERAL (
			SELECT room_id FROM seat_sessions WHERE user_id = users.id AND left_at IS NULL LIMIT 1
		) active ON true
		WHERE $1 = '%%' OR users.id ILIKE $1 OR COALESCE(users.phone, '') ILIKE $1 OR users.nickname ILIKE $1
		ORDER BY users.updated_at DESC LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := make([]operations.User, 0)
	for rows.Next() {
		var user operations.User
		if err := rows.Scan(&user.ID, &user.Phone, &user.Nickname, &user.Balance, &user.ActiveRoomID, &user.Banned, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (p *Postgres) SetUserBanned(ctx context.Context, administratorID, userID string, banned bool, reason, requestID string, now time.Time) (operations.User, bool, error) {
	if strings.TrimSpace(reason) == "" {
		return operations.User{}, false, operations.ErrReasonRequired
	}
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return operations.User{}, false, fmt.Errorf("begin user moderation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := ensureUserTx(ctx, tx, administratorID, now); err != nil {
		return operations.User{}, false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_audit_log WHERE request_id = $1)`, requestID).Scan(&exists); err != nil {
		return operations.User{}, false, fmt.Errorf("check moderation request: %w", err)
	}
	if exists {
		user, err := queryOperationsUser(ctx, tx, userID)
		return user, true, err
	}
	command, err := tx.Exec(ctx, `UPDATE users SET banned_at = CASE WHEN $2 THEN $3 ELSE NULL END, updated_at = $3 WHERE id = $1`, userID, banned, now)
	if err != nil {
		return operations.User{}, false, fmt.Errorf("update user ban status: %w", err)
	}
	if command.RowsAffected() != 1 {
		return operations.User{}, false, operations.ErrUserNotFound
	}
	action := "user.unban"
	if banned {
		action = "user.ban"
	}
	if err := insertAudit(ctx, tx, administratorID, action, "user", userID, reason, requestID, map[string]any{"banned": banned}, now); err != nil {
		return operations.User{}, false, err
	}
	user, err := queryOperationsUser(ctx, tx, userID)
	if err != nil {
		return operations.User{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return operations.User{}, false, fmt.Errorf("commit user moderation: %w", err)
	}
	return user, false, nil
}

func (p *Postgres) CreateReport(ctx context.Context, input operations.ReportInput) (operations.Report, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return operations.Report{}, false, fmt.Errorf("begin report creation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := ensureUserTx(ctx, tx, input.ReporterID, input.CreatedAt); err != nil {
		return operations.Report{}, false, err
	}
	var report operations.Report
	err = scanReport(tx.QueryRow(ctx, reportSelect+` WHERE reporter_id = $1 AND request_id = $2`, input.ReporterID, input.RequestID), &report)
	if err == nil {
		return report, true, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return operations.Report{}, false, fmt.Errorf("read report request: %w", err)
	}
	id, err := idgen.ID("report")
	if err != nil {
		return operations.Report{}, false, err
	}
	err = scanReport(tx.QueryRow(ctx, `
		INSERT INTO reports (id, reporter_id, room_id, subject_user_id, category, detail, status, request_id, created_at)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, 'open', $7, $8)
		RETURNING id, reporter_id, COALESCE(room_id, ''), COALESCE(subject_user_id, ''), category, detail,
			status, COALESCE(handled_by, ''), handled_at, created_at`,
		id, input.ReporterID, input.RoomID, input.SubjectUserID, input.Category, input.Detail, input.RequestID, input.CreatedAt), &report)
	if err != nil {
		return operations.Report{}, false, fmt.Errorf("create report: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return operations.Report{}, false, fmt.Errorf("commit report: %w", err)
	}
	return report, false, nil
}

func (p *Postgres) ListReports(ctx context.Context, status string, limit int) ([]operations.Report, error) {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, reportSelect+` WHERE $1 = '' OR status = $1 ORDER BY created_at DESC LIMIT $2`, status, clampLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()
	reports := make([]operations.Report, 0)
	for rows.Next() {
		var report operations.Report
		if err := scanReport(rows, &report); err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func (p *Postgres) HandleReport(ctx context.Context, administratorID, reportID, status, reason, requestID string, now time.Time) (operations.Report, bool, error) {
	if status != "resolved" && status != "dismissed" {
		return operations.Report{}, false, operations.ErrInvalidStatus
	}
	if strings.TrimSpace(reason) == "" {
		return operations.Report{}, false, operations.ErrReasonRequired
	}
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return operations.Report{}, false, fmt.Errorf("begin report handling: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := ensureUserTx(ctx, tx, administratorID, now); err != nil {
		return operations.Report{}, false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_audit_log WHERE request_id = $1)`, requestID).Scan(&exists); err != nil {
		return operations.Report{}, false, err
	}
	if exists {
		var report operations.Report
		err := scanReport(tx.QueryRow(ctx, reportSelect+` WHERE id = $1`, reportID), &report)
		return report, true, err
	}
	command, err := tx.Exec(ctx, `UPDATE reports SET status = $2, handled_by = $3, handled_at = $4 WHERE id = $1`, reportID, status, administratorID, now)
	if err != nil {
		return operations.Report{}, false, fmt.Errorf("handle report: %w", err)
	}
	if command.RowsAffected() != 1 {
		return operations.Report{}, false, operations.ErrReportNotFound
	}
	if err := insertAudit(ctx, tx, administratorID, "report."+status, "report", reportID, reason, requestID, map[string]any{"status": status}, now); err != nil {
		return operations.Report{}, false, err
	}
	var report operations.Report
	if err := scanReport(tx.QueryRow(ctx, reportSelect+` WHERE id = $1`, reportID), &report); err != nil {
		return operations.Report{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return operations.Report{}, false, fmt.Errorf("commit report handling: %w", err)
	}
	return report, false, nil
}

func (p *Postgres) ListAudits(ctx context.Context, limit int) ([]operations.Audit, error) {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, `
		SELECT id, administrator_id, action, target_type, COALESCE(target_id, ''), reason, request_id, metadata, created_at
		FROM admin_audit_log ORDER BY created_at DESC LIMIT $1`, clampLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list audit log: %w", err)
	}
	defer rows.Close()
	audits := make([]operations.Audit, 0)
	for rows.Next() {
		var audit operations.Audit
		var metadata []byte
		if err := rows.Scan(&audit.ID, &audit.AdministratorID, &audit.Action, &audit.TargetType, &audit.TargetID, &audit.Reason, &audit.RequestID, &metadata, &audit.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		if err := json.Unmarshal(metadata, &audit.Metadata); err != nil {
			return nil, fmt.Errorf("decode audit metadata: %w", err)
		}
		audits = append(audits, audit)
	}
	return audits, rows.Err()
}

const reportSelect = `
	SELECT id, reporter_id, COALESCE(room_id, ''), COALESCE(subject_user_id, ''), category, detail,
		status, COALESCE(handled_by, ''), handled_at, created_at
	FROM reports`

func queryOperationsUser(ctx context.Context, queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID string) (operations.User, error) {
	var user operations.User
	err := queryer.QueryRow(ctx, `
		SELECT users.id, COALESCE(users.phone, ''), users.nickname, account_score(users.id),
			COALESCE(active.room_id, ''), users.banned_at IS NOT NULL, users.created_at, users.updated_at
		FROM users
		LEFT JOIN LATERAL (
			SELECT room_id FROM seat_sessions WHERE user_id = users.id AND left_at IS NULL LIMIT 1
		) active ON true
		WHERE users.id = $1`, userID).
		Scan(&user.ID, &user.Phone, &user.Nickname, &user.Balance, &user.ActiveRoomID, &user.Banned, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func scanReport(row rowScanner, report *operations.Report) error {
	var handledAt *time.Time
	err := row.Scan(
		&report.ID, &report.ReporterID, &report.RoomID, &report.SubjectUserID, &report.Category, &report.Detail,
		&report.Status, &report.HandledBy, &handledAt, &report.CreatedAt,
	)
	if handledAt != nil {
		report.HandledAt = *handledAt
	}
	return err
}

func insertAudit(ctx context.Context, tx pgx.Tx, administratorID, action, targetType, targetID, reason, requestID string, metadata map[string]any, now time.Time) error {
	id, err := idgen.ID("audit")
	if err != nil {
		return err
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_audit_log
			(id, administrator_id, action, target_type, target_id, reason, request_id, metadata, created_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9)`,
		id, administratorID, action, targetType, targetID, reason, requestID, raw, now); err != nil {
		return fmt.Errorf("insert admin audit: %w", err)
	}
	return nil
}

func clampLimit(limit int) int {
	if limit < 1 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}
