-- +goose Up
ALTER TABLE credit_applications DROP CONSTRAINT IF EXISTS credit_applications_product_policy_id_fkey;
ALTER TABLE pricing_policies DROP CONSTRAINT IF EXISTS pricing_policies_product_policy_id_fkey;
ALTER TABLE credit_applications
  ADD COLUMN repayment_interval_days integer NOT NULL DEFAULT 30 CHECK(repayment_interval_days > 0),
  ADD COLUMN grace_days integer NOT NULL DEFAULT 0 CHECK(grace_days >= 0),
  ADD COLUMN allocation_order text[] NOT NULL DEFAULT ARRAY['penalty','fees','interest','principal']
    CHECK(cardinality(allocation_order)=4 AND allocation_order @> ARRAY['principal','interest','fees','penalty']);
COMMENT ON TABLE product_policies IS 'Legacy compatibility data only. Product Engine is the live product authority.';

-- +goose Down
ALTER TABLE credit_applications DROP COLUMN allocation_order, DROP COLUMN grace_days, DROP COLUMN repayment_interval_days;
ALTER TABLE credit_applications ADD CONSTRAINT credit_applications_product_policy_id_fkey FOREIGN KEY(product_policy_id) REFERENCES product_policies(id);
ALTER TABLE pricing_policies ADD CONSTRAINT pricing_policies_product_policy_id_fkey FOREIGN KEY(product_policy_id) REFERENCES product_policies(id);
