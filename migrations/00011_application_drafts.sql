-- +goose Up
ALTER TABLE credit_applications DROP CONSTRAINT credit_applications_status_check;
ALTER TABLE credit_applications ADD CONSTRAINT credit_applications_status_check CHECK(status IN ('draft','pending_review','awaiting_information','offered','accepted','declined','cancelled'));
CREATE TABLE credit_application_drafts (
 application_id text PRIMARY KEY REFERENCES credit_applications(id),
 caller_application_id text NOT NULL DEFAULT '',
 creation_hash text NOT NULL DEFAULT '',
 requirements jsonb NOT NULL,
 answers jsonb NOT NULL DEFAULT '{}'::jsonb,
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0)
);
CREATE TABLE credit_application_answer_history (
 application_id text NOT NULL REFERENCES credit_applications(id),
 revision integer NOT NULL,
 answers jsonb NOT NULL,
 actor text NOT NULL,
 action text NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(application_id,revision)
);
CREATE TRIGGER credit_application_answer_history_immutable BEFORE UPDATE OR DELETE ON credit_application_answer_history FOR EACH ROW EXECUTE FUNCTION prohibit_credit_audit_mutation();
-- +goose Down
-- Refuse downgrade while drafts exist rather than silently dropping applications.
ALTER TABLE credit_applications DROP CONSTRAINT credit_applications_status_check;
ALTER TABLE credit_applications ADD CONSTRAINT credit_applications_status_check CHECK(status IN ('pending_review','awaiting_information','offered','accepted','declined','cancelled'));
DROP TABLE credit_application_drafts;
DROP TABLE credit_application_answer_history;
