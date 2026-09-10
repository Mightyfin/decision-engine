-- +goose Up
-- Legacy contracts remain NULL. Never fetch today's product to backfill history.
ALTER TABLE credit_applications ADD COLUMN usage_terms jsonb;
ALTER TABLE credit_offers ADD COLUMN usage_terms jsonb;
ALTER TABLE credit_applications ADD CONSTRAINT application_usage_object
  CHECK (usage_terms IS NULL OR usage_terms = 'null'::jsonb OR jsonb_typeof(usage_terms) = 'object');
ALTER TABLE credit_offers ADD CONSTRAINT offer_usage_object
  CHECK (usage_terms IS NULL OR usage_terms = 'null'::jsonb OR jsonb_typeof(usage_terms) = 'object');

CREATE FUNCTION protect_credit_usage_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND NEW.usage_terms IS DISTINCT FROM OLD.usage_terms THEN
    RAISE EXCEPTION 'credit usage snapshot is immutable';
  END IF;
  IF TG_TABLE_NAME = 'credit_offers' THEN
    IF NOT EXISTS (
      SELECT 1 FROM credit_applications a WHERE a.id=NEW.application_id
        AND a.usage_terms IS NOT DISTINCT FROM NEW.usage_terms
    ) THEN
      RAISE EXCEPTION 'offer usage must match application snapshot';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER application_usage_immutable BEFORE UPDATE ON credit_applications
  FOR EACH ROW EXECUTE FUNCTION protect_credit_usage_snapshot();
CREATE TRIGGER offer_usage_immutable BEFORE INSERT OR UPDATE ON credit_offers
  FOR EACH ROW EXECUTE FUNCTION protect_credit_usage_snapshot();

-- +goose Down
DROP TRIGGER offer_usage_immutable ON credit_offers;
DROP TRIGGER application_usage_immutable ON credit_applications;
DROP FUNCTION protect_credit_usage_snapshot();
ALTER TABLE credit_offers DROP COLUMN usage_terms;
ALTER TABLE credit_applications DROP COLUMN usage_terms;
