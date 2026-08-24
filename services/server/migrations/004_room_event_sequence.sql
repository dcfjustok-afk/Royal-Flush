BEGIN;

ALTER TABLE room_events DROP CONSTRAINT room_events_pkey;
ALTER TABLE room_events ADD COLUMN id bigserial PRIMARY KEY;
CREATE INDEX room_events_room_version ON room_events (room_id, version, id);

COMMIT;
