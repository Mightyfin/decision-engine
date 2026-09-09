-- +goose Up
CREATE TABLE credit_application_action_replays (
 tenant_id text NOT NULL,
 environment text NOT NULL CHECK(environment IN ('sandbox','production')),
 caller_application_id text NOT NULL,
 operation text NOT NULL,
 idempotency_key text NOT NULL,
 request_hash text NOT NULL,
 response jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,environment,caller_application_id,operation,idempotency_key)
);
-- +goose Down
DROP TABLE credit_application_action_replays;
