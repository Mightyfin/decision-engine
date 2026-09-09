package storage

import (
	"context"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
)

// Publish only lifecycle facts. Internal reasons, analyst identity, evidence and
// financial handoff payloads must never become tenant webhook bodies. This runs
// in the same transaction as the application mutation and immutable audit.
func appendTenantStatusEvent(ctx context.Context, db sqlExecutor, audit creditrisk.Audit) error {
	switch audit.Action {
	case "submitted", "information_requested", "information_resubmitted", "offer", "decline", "accepted", "cancelled":
	default:
		return nil
	}
	_, err := db.Exec(ctx, `INSERT INTO credit_outbox(event_type,aggregate_id,tenant_id,payload,occurred_at)
	SELECT 'credit.application.status_changed',a.id,a.tenant_id,
	 jsonb_build_object('application_id',a.id,'tenant_id',a.tenant_id,
	 'environment',e.environment,'caller_application_id',COALESCE(NULLIF(d.caller_application_id,''),e.caller_application_id),
	 'status',a.status,'action',$2::text),$3
	FROM credit_applications a JOIN credit_application_environments e ON e.application_id=a.id
	LEFT JOIN credit_application_drafts d ON d.application_id=a.id
	WHERE a.id=$1 AND e.environment IN ('sandbox','production')
	 AND COALESCE(NULLIF(d.caller_application_id,''),e.caller_application_id)<>''`, audit.ApplicationID, audit.Action, audit.At)
	return err
}
