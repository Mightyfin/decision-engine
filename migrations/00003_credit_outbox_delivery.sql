-- +goose Up
-- Delivery state is deliberately separate from the financial/credit transition.
-- An event is recorded atomically with offer acceptance, then safely retried here.
ALTER TABLE credit_outbox
    ADD COLUMN publish_attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN publishing_at timestamptz,
    ADD COLUMN available_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN last_error text;

CREATE INDEX credit_outbox_delivery_idx
    ON credit_outbox (available_at, id)
    WHERE published_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS credit_outbox_delivery_idx;
ALTER TABLE credit_outbox
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS available_at,
    DROP COLUMN IF EXISTS publishing_at,
    DROP COLUMN IF EXISTS publish_attempts;
