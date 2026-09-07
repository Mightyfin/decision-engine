-- +goose Up
ALTER TABLE credit_applications
  DROP CONSTRAINT IF EXISTS credit_applications_origin_check;
ALTER TABLE credit_applications
  ADD CONSTRAINT credit_applications_origin_check
  CHECK (origin IS NULL OR origin ~ '^[a-z][a-z0-9_-]{0,63}$');

-- +goose Down
ALTER TABLE credit_applications
  DROP CONSTRAINT IF EXISTS credit_applications_origin_check;
ALTER TABLE credit_applications
  ADD CONSTRAINT credit_applications_origin_check
  CHECK (origin IS NULL OR origin IN ('legacy','direct_lending','embedded_finance','efaas'));
