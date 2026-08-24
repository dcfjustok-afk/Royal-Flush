BEGIN;

ALTER TABLE reports ADD COLUMN request_id text;
CREATE UNIQUE INDEX reports_idempotent_request
    ON reports (reporter_id, request_id)
    WHERE request_id IS NOT NULL;

COMMIT;
