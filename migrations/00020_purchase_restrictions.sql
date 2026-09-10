-- +goose Up
CREATE TABLE credit_purchase_restrictions (
 application_id text PRIMARY KEY REFERENCES credit_applications(id),
 order_reference text NOT NULL CHECK(length(trim(order_reference))>0),
 supplier_party_id text NOT NULL CHECK(length(trim(supplier_party_id))>0),
 destination_wallet_id text NOT NULL CHECK(length(trim(destination_wallet_id))>0),
 document_id text NOT NULL,
 sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'),
 currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 maximum_amount_minor bigint NOT NULL CHECK(maximum_amount_minor>0),
 recorded_by text NOT NULL CHECK(length(trim(recorded_by))>0),
 recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE FUNCTION protect_purchase_restriction() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'purchase restriction history is immutable'; END IF;
 PERFORM 1 FROM credit_applications WHERE id=NEW.application_id FOR UPDATE;
 IF NOT EXISTS(SELECT 1 FROM credit_applications a
  JOIN credit_application_environments x ON x.application_id=a.id
  JOIN credit_application_evidence e ON e.application_id=a.id AND e.tenant_id=a.tenant_id AND e.environment=x.environment AND e.party_id=a.party_id
  WHERE a.id=NEW.application_id AND a.status='pending_review' AND a.currency=NEW.currency
  AND NOT EXISTS(SELECT 1 FROM credit_offers o WHERE o.application_id=a.id)
  AND a.amount=NEW.maximum_amount_minor AND a.party_id<>NEW.supplier_party_id
  AND e.document_id=NEW.document_id AND e.sha256=NEW.sha256 AND e.document_type IN ('invoice','purchase_order')
 ) THEN RAISE EXCEPTION 'purchase restriction requires current application invoice evidence'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER purchase_restriction_guard BEFORE INSERT OR UPDATE OR DELETE ON credit_purchase_restrictions
 FOR EACH ROW EXECUTE FUNCTION protect_purchase_restriction();

ALTER TABLE credit_offers ADD COLUMN purchase_restriction jsonb;
CREATE FUNCTION capture_offer_purchase_restriction() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.purchase_restriction IS DISTINCT FROM OLD.purchase_restriction THEN RAISE EXCEPTION 'offer purchase restriction is immutable'; END IF;
  RETURN NEW;
 END IF;
 PERFORM 1 FROM credit_applications WHERE id=NEW.application_id FOR UPDATE;
 SELECT to_jsonb(p) INTO NEW.purchase_restriction FROM credit_purchase_restrictions p WHERE p.application_id=NEW.application_id;
 IF NEW.purchase_restriction IS NOT NULL AND NEW.principal>(NEW.purchase_restriction->>'maximum_amount_minor')::bigint THEN
  RAISE EXCEPTION 'offer exceeds purchase restriction';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER offer_purchase_restriction BEFORE INSERT OR UPDATE ON credit_offers
 FOR EACH ROW EXECUTE FUNCTION capture_offer_purchase_restriction();
-- Older acceptance writers must also fail closed until controlled execution ships.
CREATE FUNCTION block_unimplemented_purchase_acceptance() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status='accepted' AND EXISTS(SELECT 1 FROM credit_purchase_restrictions WHERE application_id=NEW.id) THEN
  RAISE EXCEPTION 'controlled purchase execution is not enabled';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER purchase_acceptance_guard BEFORE UPDATE ON credit_applications
 FOR EACH ROW EXECUTE FUNCTION block_unimplemented_purchase_acceptance();
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'purchase restriction history requires a reviewed forward migration'; END $$;
