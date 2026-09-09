package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5"
)

type commercialQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// JSONB canonicalization and ordered evidence produce the same digest across
// calls. New answers or evidence invalidate approval of an earlier snapshot.
func commercialSnapshot(ctx context.Context, q commercialQuerier, id string) (string, error) {
	var raw []byte
	err := q.QueryRow(ctx, `SELECT jsonb_build_object('answers',d.answers,'requirements',d.requirements,'amount',a.amount,'term_days',a.term_days,'product_version',a.product_policy_version,'party_id',a.party_id,'evidence',COALESCE((SELECT jsonb_agg(jsonb_build_array(e.document_id,e.sha256,e.document_type) ORDER BY e.document_id,e.sha256) FROM credit_application_evidence e WHERE e.application_id=a.id),'[]'::jsonb)) FROM credit_applications a JOIN credit_application_drafts d ON d.application_id=a.id WHERE a.id=$1`, id).Scan(&raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func commercialApproved(ctx context.Context, q commercialQuerier, id string, revision int) error {
	hash, err := commercialSnapshot(ctx, q, id)
	if err != nil {
		return err
	}
	var approved bool
	err = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credit_commercial_reviews WHERE application_id=$1 AND revision=$2 AND snapshot_hash=$3 AND decision='approved')`, id, revision, hash).Scan(&approved)
	if err != nil {
		return err
	}
	if !approved {
		return creditrisk.ErrCommercialReviewRequired
	}
	return nil
}

func (s Postgres) CommercialReview(ctx context.Context, id string, scope creditrisk.SubmissionIdentity) (creditrisk.CommercialReview, error) {
	if err := s.CheckDraftCaller(ctx, id, scope.TenantID, scope.Environment, scope.CallerApplicationID); err != nil {
		return creditrisk.CommercialReview{}, err
	}
	d, err := s.GetDraft(ctx, id, scope.TenantID, scope.Environment)
	if err != nil {
		return creditrisk.CommercialReview{}, err
	}
	hash, err := commercialSnapshot(ctx, s.Pool, id)
	if err != nil {
		return creditrisk.CommercialReview{}, err
	}
	result := creditrisk.CommercialReview{ApplicationID: id, Revision: d.Revision, Decision: "not_required", SnapshotHash: hash}
	if !d.Requirements.CommercialReviewRequired {
		return result, nil
	}
	result.Decision = "pending"
	editable := d.Application.Status == "draft" || d.Application.Status == "awaiting_information"
	err = s.Pool.QueryRow(ctx, `SELECT revision,decision,review_reference,recorded_at FROM credit_commercial_reviews WHERE application_id=$1 AND snapshot_hash=$3 AND (revision=$2 OR (NOT $4 AND revision<$2)) ORDER BY revision DESC LIMIT 1`, id, d.Revision, hash, editable).Scan(&result.Revision, &result.Decision, &result.Reference, &result.RecordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	return result, err
}

func (s Postgres) RecordCommercialReview(ctx context.Context, in creditrisk.CommercialReview, audit creditrisk.Audit, scope creditrisk.SubmissionIdentity) (result creditrisk.CommercialReview, replayed bool, err error) {
	if !validSubmissionIdentity(scope) || in.ApplicationID == "" || in.Revision < 1 || (in.Decision != "approved" && in.Decision != "rejected") || strings.TrimSpace(in.Reference) == "" || len(in.Reference) > 256 || audit.Actor == "" || len(strings.TrimSpace(audit.Reason)) < 10 || len(audit.Reason) > 4000 {
		return result, false, creditrisk.ErrInvalidState
	}
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `INSERT INTO credit_application_action_replays(tenant_id,environment,caller_application_id,operation,idempotency_key,request_hash) VALUES($1,$2,$3,'commercial_review',$4,$5) ON CONFLICT DO NOTHING`, scope.TenantID, scope.Environment, scope.CallerApplicationID, scope.Key, scope.Hash)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			var hash string
			var raw []byte
			e = tx.QueryRow(ctx, `SELECT request_hash,response FROM credit_application_action_replays WHERE tenant_id=$1 AND environment=$2 AND caller_application_id=$3 AND operation='commercial_review' AND idempotency_key=$4 FOR UPDATE`, scope.TenantID, scope.Environment, scope.CallerApplicationID, scope.Key).Scan(&hash, &raw)
			if e != nil {
				return e
			}
			if hash != scope.Hash {
				return creditrisk.ErrDraftKeyConflict
			}
			if len(raw) == 0 {
				return errors.New("commercial response missing")
			}
			e = json.Unmarshal(raw, &result)
			replayed = e == nil
			return e
		}
		var revision int
		var status string
		var required bool
		e = tx.QueryRow(ctx, `SELECT d.revision,a.status,COALESCE((d.requirements->>'commercial_review_required')::boolean,false) FROM credit_applications a JOIN credit_application_drafts d ON d.application_id=a.id JOIN credit_application_environments e ON e.application_id=a.id WHERE a.id=$1 AND a.tenant_id=$2 AND e.environment=$3 AND d.caller_application_id IN ('',$4) AND e.caller_application_id IN ('',$4) FOR UPDATE OF a,d`, in.ApplicationID, scope.TenantID, scope.Environment, scope.CallerApplicationID).Scan(&revision, &status, &required)
		if errors.Is(e, pgx.ErrNoRows) {
			return creditrisk.ErrNotFound
		}
		if e != nil {
			return e
		}
		if !required || (status != "draft" && status != "awaiting_information") || revision != in.Revision {
			return creditrisk.ErrInvalidState
		}
		hash, e := commercialSnapshot(ctx, tx, in.ApplicationID)
		if e != nil {
			return e
		}
		// The reviewer must approve precisely the version they retrieved, including
		// evidence. A concurrent evidence upload may not be silently included.
		if in.SnapshotHash != hash {
			return creditrisk.ErrInvalidState
		}
		in.RecordedAt = audit.At
		tag, e = tx.Exec(ctx, `INSERT INTO credit_commercial_reviews(application_id,revision,snapshot_hash,decision,review_reference,actor,caller_application_id,recorded_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, in.ApplicationID, in.Revision, hash, in.Decision, in.Reference, audit.Actor, scope.CallerApplicationID, audit.At)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return creditrisk.ErrInvalidState
		}
		audit.ApplicationID = in.ApplicationID
		audit.Action = "commercial_" + in.Decision
		if e = appendAudit(ctx, tx, audit); e != nil {
			return e
		}
		raw, e := json.Marshal(in)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE credit_application_action_replays SET response=$5 WHERE tenant_id=$1 AND environment=$2 AND caller_application_id=$3 AND operation='commercial_review' AND idempotency_key=$4`, scope.TenantID, scope.Environment, scope.CallerApplicationID, scope.Key, raw)
		result = in
		return e
	})
	return
}
