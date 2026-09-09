-- +goose Up
CREATE TABLE credit_offer_acceptance_replays (
 tenant_id text NOT NULL,
 environment text NOT NULL CHECK (environment IN ('sandbox','production')),
 caller_application_id text NOT NULL,
 idempotency_key text NOT NULL,
 application_id text NOT NULL REFERENCES credit_applications(id),
 request_hash text NOT NULL,
 quote_id text NOT NULL,
 consent_reference text NOT NULL,
 accepted_by text NOT NULL,
 response jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,environment,caller_application_id,idempotency_key)
);
CREATE INDEX credit_offer_acceptance_application ON credit_offer_acceptance_replays(application_id);
-- +goose Down
DROP TABLE credit_offer_acceptance_replays;
