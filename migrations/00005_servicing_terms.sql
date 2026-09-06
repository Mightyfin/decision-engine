-- +goose Up
ALTER TABLE product_policies
  ADD COLUMN repayment_interval_days integer NOT NULL DEFAULT 30 CHECK (repayment_interval_days > 0),
  ADD COLUMN grace_days integer NOT NULL DEFAULT 0 CHECK (grace_days >= 0),
  ADD COLUMN allocation_order text[] NOT NULL DEFAULT ARRAY['penalty','fees','interest','principal']
    CHECK (cardinality(allocation_order) = 4 AND allocation_order @> ARRAY['principal','interest','fees','penalty']);

ALTER TABLE pricing_policies
  ADD COLUMN penalty_rate_bps integer NOT NULL DEFAULT 0 CHECK (penalty_rate_bps >= 0),
  ADD COLUMN penalty_basis text NOT NULL DEFAULT 'overdue_principal' CHECK (penalty_basis IN ('overdue_principal','overdue_interest','overdue_principal_interest','overdue_principal_interest_fees')),
  ADD COLUMN penalty_cap_bps integer NOT NULL DEFAULT 0 CHECK (penalty_cap_bps >= 0);

ALTER TABLE credit_offers
  ADD COLUMN installment_count integer NOT NULL DEFAULT 1 CHECK (installment_count > 0),
  ADD COLUMN repayment_interval_days integer NOT NULL DEFAULT 30 CHECK (repayment_interval_days > 0),
  ADD COLUMN grace_days integer NOT NULL DEFAULT 0 CHECK (grace_days >= 0),
  ADD COLUMN penalty_rate_bps integer NOT NULL DEFAULT 0 CHECK (penalty_rate_bps >= 0),
  ADD COLUMN penalty_basis text NOT NULL DEFAULT 'overdue_principal' CHECK (penalty_basis IN ('overdue_principal','overdue_interest','overdue_principal_interest','overdue_principal_interest_fees')),
  ADD COLUMN penalty_cap_bps integer NOT NULL DEFAULT 0 CHECK (penalty_cap_bps >= 0),
  ADD COLUMN allocation_order text[] NOT NULL DEFAULT ARRAY['penalty','fees','interest','principal']
    CHECK (cardinality(allocation_order) = 4 AND allocation_order @> ARRAY['principal','interest','fees','penalty']);

-- +goose Down
ALTER TABLE credit_offers DROP COLUMN allocation_order, DROP COLUMN penalty_cap_bps, DROP COLUMN penalty_basis, DROP COLUMN penalty_rate_bps, DROP COLUMN grace_days, DROP COLUMN repayment_interval_days, DROP COLUMN installment_count;
ALTER TABLE pricing_policies DROP COLUMN penalty_cap_bps, DROP COLUMN penalty_basis, DROP COLUMN penalty_rate_bps;
ALTER TABLE product_policies DROP COLUMN allocation_order, DROP COLUMN grace_days, DROP COLUMN repayment_interval_days;
