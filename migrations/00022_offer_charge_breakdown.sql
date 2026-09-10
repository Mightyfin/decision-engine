-- +goose Up
ALTER TABLE credit_offers
  ADD COLUMN interest_method text NOT NULL DEFAULT 'legacy' CHECK (interest_method IN ('legacy','flat','reducing_balance','fixed_amount')),
  ADD COLUMN rate_period text NOT NULL DEFAULT 'legacy' CHECK (rate_period IN ('legacy','annual','monthly','term')),
  ADD COLUMN interest_rate_bps integer NOT NULL DEFAULT 0 CHECK (interest_rate_bps >= 0),
  ADD COLUMN charge_lines jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(charge_lines) = 'array');

UPDATE credit_offers
SET charge_lines =
  CASE
    WHEN fees > 0 THEN jsonb_build_array(
      jsonb_build_object('code','interest','category','interest','calculation_method','legacy','rate_period','legacy','time_convention','legacy','rounding','legacy','rate_bps',0,'amount_minor',interest),
      jsonb_build_object('code','legacy_fee','category','fee','calculation_method','legacy','rounding','legacy','amount_minor',fees)
    )
    ELSE jsonb_build_array(
      jsonb_build_object('code','interest','category','interest','calculation_method','legacy','rate_period','legacy','time_convention','legacy','rounding','legacy','rate_bps',0,'amount_minor',interest)
    )
  END;

-- +goose Down
ALTER TABLE credit_offers
  DROP COLUMN charge_lines,
  DROP COLUMN interest_rate_bps,
  DROP COLUMN rate_period,
  DROP COLUMN interest_method;
