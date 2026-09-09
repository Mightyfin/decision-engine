-- +goose Up
CREATE INDEX credit_applications_tenant_id_idx ON credit_applications(tenant_id,id);
CREATE INDEX credit_applications_tenant_relationship_id_idx ON credit_applications(tenant_id,relationship_id,id);
CREATE INDEX credit_decision_audit_application_id_idx ON credit_decision_audit(application_id,id);
-- +goose Down
DROP INDEX credit_decision_audit_application_id_idx;
DROP INDEX credit_applications_tenant_relationship_id_idx;
DROP INDEX credit_applications_tenant_id_idx;
