ALTER TABLE pricing_policies
  ADD COLUMN IF NOT EXISTS interest_method text NOT NULL DEFAULT 'flat',
  ADD COLUMN IF NOT EXISTS rate_period text NOT NULL DEFAULT 'annual',
  ADD COLUMN IF NOT EXISTS interest_rate_bps integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS fixed_interest bigint NOT NULL DEFAULT 0;
UPDATE pricing_policies SET interest_rate_bps=annual_rate_bps WHERE interest_rate_bps=0;
ALTER TABLE pricing_policies DROP CONSTRAINT IF EXISTS pricing_policies_penalty_basis_check;
ALTER TABLE pricing_policies ADD CONSTRAINT pricing_policies_penalty_basis_check CHECK (penalty_basis IN (
 'overdue_principal','overdue_interest','overdue_principal_interest','overdue_principal_interest_fees','overdue_principal_interest_penalty','overdue_principal_interest_fees_penalty','overdue_interest_fees','overdue_fees','overdue_penalty','overdue_principal_fees','overdue_principal_penalty','overdue_interest_penalty','overdue_fees_penalty','overdue_principal_fees_penalty','overdue_interest_fees_penalty','total_principal_released','total_principal_balance'));
ALTER TABLE pricing_policies ADD CONSTRAINT pricing_policies_interest_method_check CHECK (interest_method IN ('flat','reducing_balance','fixed_amount'));
ALTER TABLE pricing_policies ADD CONSTRAINT pricing_policies_rate_period_check CHECK (rate_period IN ('annual','monthly','term'));
