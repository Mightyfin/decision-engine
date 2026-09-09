-- +goose Up
ALTER TABLE credit_application_environments ADD COLUMN caller_application_id text NOT NULL DEFAULT '';

-- Recover only provenance already recorded by durable submissions. Ambiguous
-- ownership aborts migration rather than granting tenant-wide access.
DO $$ BEGIN
 IF EXISTS (
  SELECT response->>'id' FROM credit_submission_replays
  WHERE response IS NOT NULL
  GROUP BY response->>'id' HAVING count(DISTINCT caller_application_id)>1
 ) THEN RAISE EXCEPTION 'Ambiguous credit submission ownership requires review'; END IF;
END $$;
-- The pre-existing scope trigger rejects every UPDATE. Pause it only inside
-- this migration transaction, while the table's DDL lock excludes writers.
-- Failure rolls back both the backfill and trigger state.
ALTER TABLE credit_application_environments DISABLE TRIGGER credit_environment_immutable;
UPDATE credit_application_environments e
SET caller_application_id=r.caller_application_id
FROM credit_submission_replays r, credit_applications a
WHERE r.response->>'id'=e.application_id AND a.id=e.application_id
 AND a.tenant_id=r.tenant_id AND e.environment=r.environment;
ALTER TABLE credit_application_environments ENABLE TRIGGER credit_environment_immutable;

CREATE INDEX credit_application_environment_owner_idx
 ON credit_application_environments(environment,caller_application_id,application_id);

-- +goose Down
DROP INDEX credit_application_environment_owner_idx;
ALTER TABLE credit_application_environments DROP COLUMN caller_application_id;
