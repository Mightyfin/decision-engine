-- +goose Up
CREATE TABLE credit_outbox (
  id bigserial PRIMARY KEY,
  event_type text NOT NULL,
  aggregate_id text NOT NULL REFERENCES credit_applications(id),
  tenant_id text NOT NULL,
  payload jsonb NOT NULL,
  occurred_at timestamptz NOT NULL,
  published_at timestamptz NULL
);
CREATE INDEX credit_outbox_unpublished_idx ON credit_outbox(published_at, id) WHERE published_at IS NULL;
-- +goose Down
DROP TABLE credit_outbox;
