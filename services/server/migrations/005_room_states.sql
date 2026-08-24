BEGIN;

CREATE TABLE room_states (
    room_id text PRIMARY KEY REFERENCES rooms(id) ON DELETE CASCADE,
    state jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

COMMIT;
