-- +goose Up
ALTER TABLE pricing_policies ADD COLUMN configured_currency text NOT NULL DEFAULT ''
 CHECK (configured_currency = '' OR configured_currency ~ '^[A-Z]{3}$');
CREATE TABLE credit_default_pricing_bindings (
 tenant_id text NOT NULL CHECK (tenant_id <> '' AND tenant_id <> '__mightyfin_default__'),
 product_policy_id text NOT NULL CHECK (product_policy_id <> ''),
 default_policy_key text NOT NULL CHECK (default_policy_key <> ''),
 currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
 created_by text NOT NULL CHECK (created_by <> ''),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id, product_policy_id)
);
CREATE TABLE credit_pricing_configuration_audit (
 id bigserial PRIMARY KEY,
 tenant_id text NOT NULL,
 product_policy_id text NOT NULL,
 actor text NOT NULL,
 action text NOT NULL,
 configuration jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER credit_pricing_configuration_audit_immutable
 BEFORE UPDATE OR DELETE ON credit_pricing_configuration_audit
 FOR EACH ROW EXECUTE FUNCTION prohibit_credit_audit_mutation();

-- +goose Down
DROP TABLE credit_pricing_configuration_audit, credit_default_pricing_bindings;
ALTER TABLE pricing_policies DROP COLUMN configured_currency;
