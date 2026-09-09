-- +goose Up
CREATE TABLE credit_commercial_reviews (
 application_id text NOT NULL REFERENCES credit_applications(id),
 revision integer NOT NULL CHECK(revision>0),
 snapshot_hash text NOT NULL,
 decision text NOT NULL CHECK(decision IN ('approved','rejected')),
 review_reference text NOT NULL,
 actor text NOT NULL,
 caller_application_id text NOT NULL,
 recorded_at timestamptz NOT NULL,
 PRIMARY KEY(application_id,revision,snapshot_hash)
);
CREATE TRIGGER credit_commercial_reviews_immutable BEFORE UPDATE OR DELETE ON credit_commercial_reviews FOR EACH ROW EXECUTE FUNCTION prohibit_credit_audit_mutation();

-- +goose Down
DROP TABLE credit_commercial_reviews;
