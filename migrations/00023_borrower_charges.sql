-- +goose Up
ALTER TABLE pricing_policies
  ADD COLUMN borrower_charges jsonb NOT NULL DEFAULT '[]'::jsonb
  CHECK (jsonb_typeof(borrower_charges) = 'array');

-- +goose Down
ALTER TABLE pricing_policies DROP COLUMN borrower_charges;
