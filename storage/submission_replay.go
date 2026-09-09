package storage

import (
	"context"
	"encoding/json"
	"errors"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5"
)

func validSubmissionIdentity(in creditrisk.SubmissionIdentity) bool {
	return in.TenantID != "" && (in.Environment == "sandbox" || in.Environment == "production") && in.CallerApplicationID != "" && in.Key != "" && in.Hash != ""
}

func (s Postgres) SubmissionReplay(ctx context.Context, in creditrisk.SubmissionIdentity) (creditrisk.Application, bool, error) {
	if !validSubmissionIdentity(in) {
		return creditrisk.Application{}, false, creditrisk.ErrInvalidState
	}
	var hash string
	var encoded []byte
	err := s.Pool.QueryRow(ctx, `SELECT request_hash,response FROM credit_submission_replays WHERE tenant_id=$1 AND environment=$2 AND caller_application_id=$3 AND idempotency_key=$4`, in.TenantID, in.Environment, in.CallerApplicationID, in.Key).Scan(&hash, &encoded)
	if errors.Is(err, pgx.ErrNoRows) {
		return creditrisk.Application{}, false, nil
	}
	if err != nil {
		return creditrisk.Application{}, false, err
	}
	a, err := decodeSubmissionReplay(hash, encoded, in)
	return a, err == nil, err
}

func decodeSubmissionReplay(hash string, encoded []byte, in creditrisk.SubmissionIdentity) (a creditrisk.Application, err error) {
	if hash != in.Hash {
		return a, creditrisk.ErrDraftKeyConflict
	}
	if len(encoded) == 0 {
		return a, errors.New("submission response not committed")
	}
	err = json.Unmarshal(encoded, &a)
	return
}

func (s Postgres) CreateSubmission(ctx context.Context, a creditrisk.Application, audit creditrisk.Audit, in creditrisk.SubmissionIdentity) (result creditrisk.Application, err error) {
	if !validSubmissionIdentity(in) || a.TenantID != in.TenantID || a.Environment != in.Environment || a.Status != "pending_review" || audit.ApplicationID != a.ID || audit.Actor == "" {
		return result, creditrisk.ErrInvalidState
	}
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `INSERT INTO credit_submission_replays(tenant_id,environment,caller_application_id,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, in.TenantID, in.Environment, in.CallerApplicationID, in.Key, in.Hash)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			var hash string
			var encoded []byte
			e = tx.QueryRow(ctx, `SELECT request_hash,response FROM credit_submission_replays WHERE tenant_id=$1 AND environment=$2 AND caller_application_id=$3 AND idempotency_key=$4 FOR UPDATE`, in.TenantID, in.Environment, in.CallerApplicationID, in.Key).Scan(&hash, &encoded)
			if e != nil {
				return e
			}
			result, e = decodeSubmissionReplay(hash, encoded, in)
			return e
		}
		if e = saveApplication(ctx, tx, a); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO credit_application_environments(application_id,environment,caller_application_id) VALUES($1,$2,$3)`, a.ID, a.Environment, in.CallerApplicationID); e != nil {
			return e
		}
		if e = appendAudit(ctx, tx, audit); e != nil {
			return e
		}
		encoded, e := json.Marshal(a)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE credit_submission_replays SET response=$5 WHERE tenant_id=$1 AND environment=$2 AND caller_application_id=$3 AND idempotency_key=$4`, in.TenantID, in.Environment, in.CallerApplicationID, in.Key, encoded)
		result = a
		return e
	})
	return
}
