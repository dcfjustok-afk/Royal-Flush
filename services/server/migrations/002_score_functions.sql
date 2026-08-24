BEGIN;

CREATE OR REPLACE FUNCTION current_score_epoch_id() RETURNS bigint
LANGUAGE sql STABLE AS $$
    SELECT id FROM score_epochs ORDER BY id DESC LIMIT 1
$$;

CREATE OR REPLACE FUNCTION account_score(account_id text) RETURNS bigint
LANGUAGE sql STABLE AS $$
    WITH current_epoch AS (
        SELECT id, base_score FROM score_epochs ORDER BY id DESC LIMIT 1
    )
    SELECT current_epoch.base_score + COALESCE(SUM(score_ledger.amount), 0)
    FROM current_epoch
    LEFT JOIN score_ledger
      ON score_ledger.epoch_id = current_epoch.id
     AND score_ledger.user_id = account_id
    GROUP BY current_epoch.base_score
$$;

COMMIT;

