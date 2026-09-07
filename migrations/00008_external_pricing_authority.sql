ALTER TABLE credit_applications ADD COLUMN IF NOT EXISTS pricing_policy_id text;
COMMENT ON COLUMN credit_applications.pricing_policy_id IS 'Immutable Pricing Engine policy reference snapshotted from the Product Engine version at submission.';
COMMENT ON TABLE pricing_policies IS 'Legacy compatibility data only. New pricing authority is the standalone Pricing Engine.';
