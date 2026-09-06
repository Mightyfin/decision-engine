-- +goose Up
-- These references identify the applicant without embedding Party or Wallet data
-- in the credit domain. Existing cases remain explicitly unverified until a
-- future subject-verifier migration can validate their source references.
ALTER TABLE credit_applications
    ADD COLUMN party_id text,
    ADD COLUMN applicant_role text CHECK (applicant_role IS NULL OR applicant_role IN ('network_participant','partner_organisation')),
    ADD COLUMN wallet_id text,
    ADD COLUMN origin text CHECK (origin IS NULL OR origin IN ('direct_lending','embedded_finance','efaas'));

CREATE INDEX credit_applications_party_idx ON credit_applications (party_id) WHERE party_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS credit_applications_party_idx;
ALTER TABLE credit_applications
    DROP COLUMN IF EXISTS origin,
    DROP COLUMN IF EXISTS wallet_id,
    DROP COLUMN IF EXISTS applicant_role,
    DROP COLUMN IF EXISTS party_id;
