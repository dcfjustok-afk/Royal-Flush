package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/royal-flush/royal-flush/services/server/internal/auth"
)

func (p *Postgres) SaveUser(ctx context.Context, record auth.StoredUser) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	permissions, err := json.Marshal(record.Permissions)
	if err != nil {
		return fmt.Errorf("encode user permissions: %w", err)
	}
	_, err = p.pool.Exec(ctx, `
		INSERT INTO users (id, phone, nickname, password_hash, permissions, created_at, updated_at)
		VALUES ($1, NULLIF($2, ''), $3, NULLIF($4, ''), $5, $6, now())
		ON CONFLICT (id) DO UPDATE SET
			phone = COALESCE(NULLIF(EXCLUDED.phone, ''), users.phone),
			nickname = EXCLUDED.nickname,
			password_hash = COALESCE(NULLIF(EXCLUDED.password_hash, ''), users.password_hash),
			permissions = EXCLUDED.permissions,
			updated_at = now()`,
		record.ID, record.Phone, record.Nickname, record.PasswordHash, permissions, record.CreatedAt)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return auth.ErrPhoneRegistered
		}
		return fmt.Errorf("save auth user: %w", err)
	}
	return nil
}

func (p *Postgres) UserByPhone(ctx context.Context, phone string) (auth.StoredUser, bool, error) {
	return p.queryAuthUser(ctx, `
		SELECT id, COALESCE(phone, ''), nickname, COALESCE(password_hash, ''), permissions,
			banned_at IS NOT NULL, created_at
		FROM users WHERE phone = $1`, phone)
}

func (p *Postgres) UserByID(ctx context.Context, userID string) (auth.StoredUser, bool, error) {
	return p.queryAuthUser(ctx, `
		SELECT id, COALESCE(phone, ''), nickname, COALESCE(password_hash, ''), permissions,
			banned_at IS NOT NULL, created_at
		FROM users WHERE id = $1`, userID)
}

func (p *Postgres) SaveSession(ctx context.Context, tokenHash, userID string, expiresAt time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	if _, err := p.pool.Exec(ctx, `
		INSERT INTO auth_sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (token_hash) DO UPDATE SET user_id = EXCLUDED.user_id, expires_at = EXCLUDED.expires_at`,
		tokenHash, userID, expiresAt); err != nil {
		return fmt.Errorf("save auth session: %w", err)
	}
	return nil
}

func (p *Postgres) UserBySession(ctx context.Context, tokenHash string, now time.Time) (auth.StoredUser, bool, error) {
	return p.queryAuthUser(ctx, `
		SELECT users.id, COALESCE(users.phone, ''), users.nickname, COALESCE(users.password_hash, ''), users.permissions,
			users.banned_at IS NOT NULL, users.created_at
		FROM auth_sessions
		JOIN users ON users.id = auth_sessions.user_id
		WHERE auth_sessions.token_hash = $1 AND auth_sessions.expires_at > $2`, tokenHash, now)
}

func (p *Postgres) DeleteSession(ctx context.Context, tokenHash string) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	if _, err := p.pool.Exec(ctx, `DELETE FROM auth_sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete auth session: %w", err)
	}
	return nil
}

func (p *Postgres) SetBanned(ctx context.Context, userID string, banned bool) error {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	command, err := p.pool.Exec(ctx, `
		UPDATE users SET banned_at = CASE WHEN $2 THEN COALESCE(banned_at, now()) ELSE NULL END, updated_at = now()
		WHERE id = $1`, userID, banned)
	if err != nil {
		return fmt.Errorf("update auth ban status: %w", err)
	}
	if command.RowsAffected() == 0 {
		return auth.ErrUserNotFound
	}
	return nil
}

func (p *Postgres) queryAuthUser(ctx context.Context, query string, arguments ...any) (auth.StoredUser, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	var record auth.StoredUser
	var permissions []byte
	err := p.pool.QueryRow(ctx, query, arguments...).Scan(
		&record.ID, &record.Phone, &record.Nickname, &record.PasswordHash, &permissions, &record.Banned, &record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.StoredUser{}, false, nil
	}
	if err != nil {
		return auth.StoredUser{}, false, fmt.Errorf("query auth user: %w", err)
	}
	if err := json.Unmarshal(permissions, &record.Permissions); err != nil {
		return auth.StoredUser{}, false, fmt.Errorf("decode user permissions: %w", err)
	}
	if record.Permissions == nil {
		record.Permissions = map[string]bool{}
	}
	return record, true, nil
}
