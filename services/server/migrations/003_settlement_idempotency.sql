BEGIN;

CREATE UNIQUE INDEX score_ledger_unique_settlement
    ON score_ledger (request_id)
    WHERE entry_type = 'game_settlement';

COMMIT;
