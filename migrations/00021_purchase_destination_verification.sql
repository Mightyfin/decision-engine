-- +goose Up
-- Legacy restrictions remain without a proof and cannot be executed.
ALTER TABLE credit_purchase_restrictions ADD COLUMN destination_verification jsonb;
CREATE FUNCTION require_purchase_destination_verification() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE env text;
BEGIN
 SELECT environment INTO env FROM credit_application_environments WHERE application_id=NEW.application_id;
 IF NEW.destination_verification IS NULL
 OR NEW.destination_verification->>'wallet_id' IS DISTINCT FROM NEW.destination_wallet_id
 OR NEW.destination_verification->>'party_id' IS DISTINCT FROM NEW.supplier_party_id
 OR NEW.destination_verification->>'currency' IS DISTINCT FROM NEW.currency
 OR NEW.destination_verification->>'environment' IS DISTINCT FROM env
 OR NEW.destination_verification->>'tenant_id' IS DISTINCT FROM (SELECT tenant_id FROM credit_applications WHERE id=NEW.application_id)
 OR NEW.destination_verification->>'verification' IS DISTINCT FROM 'active_owned_wallet'
 OR coalesce(NEW.destination_verification->>'legal_entity_id','')=''
 OR (coalesce((NEW.destination_verification->>'verified_at')::timestamptz,'epoch'::timestamptz) NOT BETWEEN clock_timestamp()-interval '1 minute' AND clock_timestamp()+interval '5 seconds') THEN
  RAISE EXCEPTION 'current wallet destination verification required';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER purchase_destination_guard BEFORE INSERT ON credit_purchase_restrictions
 FOR EACH ROW EXECUTE FUNCTION require_purchase_destination_verification();
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'destination verification history requires a reviewed forward migration'; END $$;
