BEGIN;

CREATE TABLE users (
    id text PRIMARY KEY,
    phone text UNIQUE,
    nickname text NOT NULL,
    banned_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE score_epochs (
    id bigserial PRIMARY KEY,
    base_score bigint NOT NULL DEFAULT 1000,
    administrator_id text REFERENCES users(id),
    reason text NOT NULL,
    request_id text UNIQUE,
    affected_users integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (base_score = 1000)
);

INSERT INTO score_epochs (base_score, reason) VALUES (1000, 'initial epoch');

CREATE TABLE rooms (
    id text PRIMARY KEY,
    code text NOT NULL UNIQUE,
    owner_id text NOT NULL REFERENCES users(id),
    config jsonb NOT NULL,
    status text NOT NULL DEFAULT 'waiting',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    empty_since timestamptz,
    ended_at timestamptz,
    CHECK (status IN ('waiting', 'playing', 'empty', 'ended'))
);

CREATE TABLE seat_sessions (
    id text PRIMARY KEY,
    room_id text NOT NULL REFERENCES rooms(id),
    user_id text NOT NULL REFERENCES users(id),
    seat smallint NOT NULL CHECK (seat BETWEEN 0 AND 7),
    allocated_points bigint NOT NULL DEFAULT 1000,
    remaining_points bigint,
    joined_at timestamptz NOT NULL DEFAULT now(),
    left_at timestamptz,
    CHECK (allocated_points >= 1000)
);

CREATE UNIQUE INDEX seat_sessions_one_active_room_per_user
    ON seat_sessions (user_id) WHERE left_at IS NULL;
CREATE UNIQUE INDEX seat_sessions_one_active_seat_per_room
    ON seat_sessions (room_id, seat) WHERE left_at IS NULL;

CREATE TABLE score_ledger (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id),
    epoch_id bigint NOT NULL REFERENCES score_epochs(id),
    entry_type text NOT NULL,
    amount bigint NOT NULL,
    balance_after bigint NOT NULL,
    room_id text REFERENCES rooms(id),
    request_id text,
    note text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (entry_type IN ('initial_base', 'self_add', 'game_settlement', 'admin_reset'))
);

CREATE UNIQUE INDEX score_ledger_idempotent_request
    ON score_ledger (user_id, request_id) WHERE request_id IS NOT NULL;

CREATE TABLE score_addition_requests (
    user_id text NOT NULL REFERENCES users(id),
    request_id text NOT NULL,
    ledger_id text NOT NULL REFERENCES score_ledger(id),
    amount bigint NOT NULL CHECK (amount BETWEEN 1 AND 1000000000),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, request_id)
);

CREATE INDEX score_addition_rate_limit
    ON score_addition_requests (user_id, created_at DESC);

CREATE TABLE seat_settlements (
    seat_session_id text PRIMARY KEY REFERENCES seat_sessions(id),
    ledger_id text NOT NULL UNIQUE REFERENCES score_ledger(id),
    net_points bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE room_events (
    room_id text NOT NULL REFERENCES rooms(id),
    version bigint NOT NULL,
    event_type text NOT NULL,
    request_id text,
    actor_user_id text REFERENCES users(id),
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, version)
);

CREATE UNIQUE INDEX room_events_idempotent_command
    ON room_events (room_id, actor_user_id, request_id)
    WHERE request_id IS NOT NULL AND actor_user_id IS NOT NULL;

CREATE TABLE reports (
    id text PRIMARY KEY,
    reporter_id text NOT NULL REFERENCES users(id),
    room_id text REFERENCES rooms(id),
    subject_user_id text REFERENCES users(id),
    category text NOT NULL,
    detail text NOT NULL,
    status text NOT NULL DEFAULT 'open',
    handled_by text REFERENCES users(id),
    handled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (status IN ('open', 'reviewing', 'resolved', 'dismissed'))
);

CREATE TABLE admin_audit_log (
    id text PRIMARY KEY,
    administrator_id text NOT NULL REFERENCES users(id),
    action text NOT NULL,
    target_type text NOT NULL,
    target_id text,
    reason text NOT NULL,
    request_id text NOT NULL UNIQUE,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

COMMIT;

