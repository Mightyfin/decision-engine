-- +goose Up
-- Existing offers stay NULL: do not invent historical review evidence.
ALTER TABLE credit_offers ADD COLUMN evidence_snapshot jsonb;
ALTER TABLE credit_offers ADD CONSTRAINT offer_evidence_array CHECK(evidence_snapshot IS NULL OR jsonb_typeof(evidence_snapshot)='array');

CREATE FUNCTION capture_offer_evidence_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.evidence_snapshot IS DISTINCT FROM OLD.evidence_snapshot THEN
   RAISE EXCEPTION 'offer evidence snapshot is immutable';
  END IF;
  RETURN NEW;
 END IF;
 -- BindEvidence and staff decisions serialize on this same application.
 PERFORM 1 FROM credit_applications WHERE id=NEW.application_id FOR UPDATE;
 -- Never trust a supplied snapshot. Only capture evidence from the owning tables.
 SELECT coalesce(jsonb_agg(jsonb_build_object(
  'application_id',e.application_id,'document_id',e.document_id,'sha256',e.sha256,
  'tenant_id',e.tenant_id,'environment',e.environment,'party_id',e.party_id,
  'document_type',e.document_type,'linked_by',e.linked_by,'linked_at',e.linked_at
 ) ORDER BY e.document_id,e.sha256),'[]'::jsonb) INTO NEW.evidence_snapshot
 FROM credit_application_evidence e
 JOIN credit_applications a ON a.id=e.application_id AND a.tenant_id=e.tenant_id AND a.party_id=e.party_id
 JOIN credit_application_environments x ON x.application_id=a.id AND x.environment=e.environment
 WHERE e.application_id=NEW.application_id;
 RETURN NEW;
END;
$$;
CREATE TRIGGER offer_evidence_snapshot BEFORE INSERT OR UPDATE ON credit_offers
 FOR EACH ROW EXECUTE FUNCTION capture_offer_evidence_snapshot();

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'offer evidence history requires a reviewed forward migration'; END $$;
