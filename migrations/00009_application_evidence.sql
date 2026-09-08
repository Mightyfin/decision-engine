-- +goose Up
-- Historical environments are unknown: deliberately no blanket backfill.
CREATE TABLE credit_application_environments (
 application_id text PRIMARY KEY REFERENCES credit_applications(id),
 environment text NOT NULL CHECK(environment IN ('local','dev','sandbox','staging','production'))
);
CREATE TABLE credit_application_evidence (
 application_id text NOT NULL REFERENCES credit_applications(id),
 document_id text NOT NULL,
 sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'),
 tenant_id text NOT NULL,
 environment text NOT NULL,
 party_id text NOT NULL,
 document_type text NOT NULL,
 linked_by text NOT NULL,
 linked_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(application_id,document_id,sha256)
);
-- +goose StatementBegin
CREATE FUNCTION reject_credit_evidence_mutation() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'credit evidence scope and bindings are append-only'; END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER credit_environment_immutable BEFORE UPDATE OR DELETE ON credit_application_environments FOR EACH ROW EXECUTE FUNCTION reject_credit_evidence_mutation();
CREATE TRIGGER credit_evidence_immutable BEFORE UPDATE OR DELETE ON credit_application_evidence FOR EACH ROW EXECUTE FUNCTION reject_credit_evidence_mutation();
-- +goose Down
DROP TABLE credit_application_evidence;
DROP TABLE credit_application_environments;
DROP FUNCTION reject_credit_evidence_mutation();
