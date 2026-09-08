-- +goose Up
ALTER TABLE credit_applications DROP CONSTRAINT credit_applications_status_check;
ALTER TABLE credit_applications ADD CONSTRAINT credit_applications_status_check CHECK(status IN ('pending_review','awaiting_information','offered','accepted','declined','cancelled'));
-- +goose Down
-- Downgrade deliberately fails if information requests remain open.
ALTER TABLE credit_applications DROP CONSTRAINT credit_applications_status_check;
ALTER TABLE credit_applications ADD CONSTRAINT credit_applications_status_check CHECK(status IN ('pending_review','offered','accepted','declined','cancelled'));
