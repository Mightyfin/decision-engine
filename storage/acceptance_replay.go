package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5"
)

func (s Postgres) AcceptOffer(ctx context.Context, a creditrisk.Application, offer creditrisk.Offer, audit creditrisk.Audit, in creditrisk.AcceptanceRequest) (result creditrisk.Application, replayed bool, err error) {
	if a.ID == "" || a.TenantID != in.TenantID || a.Environment != in.Environment || audit.Actor == "" || audit.ApplicationID != a.ID || in.CallerApplicationID == "" || in.Key == "" {
		return result, false, creditrisk.ErrInvalidState
	}
	payload, _ := json.Marshal([]string{a.ID, in.QuoteID, in.ConsentReference})
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		// Concurrent equal keys wait for the committed response; failed attempts
		// roll back the claim, so no permanent 'processing' record remains.
		tag, e := tx.Exec(ctx, `INSERT INTO credit_offer_acceptance_replays(tenant_id,environment,caller_application_id,idempotency_key,application_id,request_hash,quote_id,consent_reference,accepted_by)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`, in.TenantID, in.Environment, in.CallerApplicationID, in.Key, a.ID, hash, in.QuoteID, in.ConsentReference, audit.Actor)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			var previousHash string
			var response []byte
			e = tx.QueryRow(ctx, `SELECT request_hash,response FROM credit_offer_acceptance_replays WHERE tenant_id=$1 AND environment=$2 AND caller_application_id=$3 AND idempotency_key=$4 FOR UPDATE`, in.TenantID, in.Environment, in.CallerApplicationID, in.Key).Scan(&previousHash, &response)
			if e != nil {
				return e
			}
			if previousHash != hash {
				return creditrisk.ErrAcceptanceConflict
			}
			if len(response) == 0 {
				return errors.New("acceptance response not committed")
			}
			if e = json.Unmarshal(response, &result); e != nil {
				return e
			}
			replayed = true
			return nil
		}
		if offer.ApplicationID != a.ID || offer.QuoteID != in.QuoteID || in.ConsentReference == "" {
			return creditrisk.ErrInvalidState
		}
		var environment, submissionOwner, draftOwner string
		if e = tx.QueryRow(ctx, `SELECT e.environment,e.caller_application_id,COALESCE(d.caller_application_id,'') FROM credit_application_environments e LEFT JOIN credit_application_drafts d ON d.application_id=e.application_id WHERE e.application_id=$1`, a.ID).Scan(&environment, &submissionOwner, &draftOwner); e != nil {
			return e
		}
		if environment != in.Environment || (submissionOwner != "" && submissionOwner != in.CallerApplicationID) || (draftOwner != "" && draftOwner != in.CallerApplicationID) {
			return creditrisk.ErrNotFound
		}
		a.Status = "accepted"
		if e = recordAcceptanceWithEvent(ctx, tx, a, audit, offer); e != nil {
			return e
		}
		encoded, e := json.Marshal(a)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE credit_offer_acceptance_replays SET response=$5 WHERE tenant_id=$1 AND environment=$2 AND caller_application_id=$3 AND idempotency_key=$4`, in.TenantID, in.Environment, in.CallerApplicationID, in.Key, encoded)
		result = a
		return e
	})
	return
}
