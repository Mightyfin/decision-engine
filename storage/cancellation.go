package storage

import (
	"context"
	"encoding/json"
	"strings"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5"
)

func (s Postgres) CancelApplication(ctx context.Context, a creditrisk.Application, audit creditrisk.Audit, in creditrisk.SubmissionIdentity) (result creditrisk.Application, replayed bool, err error) {
	if !validSubmissionIdentity(in) || a.TenantID != in.TenantID || a.Environment != in.Environment || audit.ApplicationID != a.ID || audit.Actor == "" || len(strings.TrimSpace(audit.Reason)) < 10 || len(audit.Reason) > 4000 {
		return result, false, creditrisk.ErrInvalidState
	}
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `INSERT INTO credit_application_action_replays(tenant_id,environment,caller_application_id,operation,idempotency_key,request_hash) VALUES($1,$2,$3,'cancel',$4,$5) ON CONFLICT DO NOTHING`, in.TenantID, in.Environment, in.CallerApplicationID, in.Key, in.Hash)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			var hash string
			var encoded []byte
			e = tx.QueryRow(ctx, `SELECT request_hash,response FROM credit_application_action_replays WHERE tenant_id=$1 AND environment=$2 AND caller_application_id=$3 AND operation='cancel' AND idempotency_key=$4 FOR UPDATE`, in.TenantID, in.Environment, in.CallerApplicationID, in.Key).Scan(&hash, &encoded)
			if e != nil {
				return e
			}
			result, e = decodeSubmissionReplay(hash, encoded, in)
			replayed = e == nil
			return e
		}
		// This serializes against analyst review and acceptance. Accepted applications
		// can already have a funding handoff; they require an operational unwind,
		// not a tenant cancellation that silently discards obligations.
		tag, e = tx.Exec(ctx, `UPDATE credit_applications a SET status='cancelled' WHERE a.id=$1 AND a.tenant_id=$2 AND a.status IN ('draft','pending_review','awaiting_information','offered')
   AND EXISTS(SELECT 1 FROM credit_application_environments e WHERE e.application_id=a.id AND e.environment=$3 AND e.caller_application_id IN ('',$4))
   AND NOT EXISTS(SELECT 1 FROM credit_application_drafts d WHERE d.application_id=a.id AND d.caller_application_id<>$4)`, a.ID, in.TenantID, in.Environment, in.CallerApplicationID)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return creditrisk.ErrInvalidState
		}
		audit.Action = "cancelled"
		if e = appendAudit(ctx, tx, audit); e != nil {
			return e
		}
		event, _ := json.Marshal(map[string]string{"application_id": a.ID, "tenant_id": a.TenantID, "environment": in.Environment, "status": "cancelled"})
		if _, e = tx.Exec(ctx, `INSERT INTO credit_outbox(event_type,aggregate_id,tenant_id,payload,occurred_at) VALUES('credit.application.cancelled',$1,$2,$3,$4)`, a.ID, a.TenantID, event, audit.At); e != nil {
			return e
		}
		a.Status = "cancelled"
		encoded, e := json.Marshal(a)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE credit_application_action_replays SET response=$5 WHERE tenant_id=$1 AND environment=$2 AND caller_application_id=$3 AND operation='cancel' AND idempotency_key=$4`, in.TenantID, in.Environment, in.CallerApplicationID, in.Key, encoded)
		result = a
		return e
	})
	return
}
