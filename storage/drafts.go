package storage

import (
	"context"
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5"
)

func (s Postgres) CreateDraft(ctx context.Context, d creditrisk.Draft, audit creditrisk.Audit) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, d.Application.ID); err != nil {
			return err
		}
		var owner, hash string
		err := tx.QueryRow(ctx, `SELECT caller_application_id,creation_hash FROM credit_application_drafts WHERE application_id=$1`, d.Application.ID).Scan(&owner, &hash)
		if err == nil {
			if d.CreationHash == "" || hash != d.CreationHash || owner != d.CallerApplicationID {
				return creditrisk.ErrDraftKeyConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := saveApplication(ctx, tx, d.Application); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO credit_application_environments(application_id,environment) VALUES($1,$2)`, d.Application.ID, d.Application.Environment); err != nil {
			return err
		}
		raw, err := json.Marshal(d.Requirements)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO credit_application_drafts(application_id,requirements,caller_application_id,creation_hash) VALUES($1,$2,$3,$4)`, d.Application.ID, raw, d.CallerApplicationID, d.CreationHash); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO credit_application_answer_history(application_id,revision,answers,actor,action) VALUES($1,1,'{}'::jsonb,$2,$3)`, d.Application.ID, audit.Actor, audit.Action); err != nil {
			return err
		}
		return appendAudit(ctx, tx, audit)
	})
}
func (s Postgres) GetDraft(ctx context.Context, id, tenant, environment string) (creditrisk.Draft, error) {
	if tenant == "" || environment == "" {
		return creditrisk.Draft{}, creditrisk.ErrNotFound
	}
	var d creditrisk.Draft
	var requirements, answers []byte
	err := s.Pool.QueryRow(ctx, `SELECT d.requirements,d.answers,d.revision,d.caller_application_id,d.creation_hash FROM credit_application_drafts d JOIN credit_applications a ON a.id=d.application_id JOIN credit_application_environments e ON e.application_id=a.id WHERE a.id=$1 AND a.tenant_id=$2 AND e.environment=$3`, id, tenant, environment).Scan(&requirements, &answers, &d.Revision, &d.CallerApplicationID, &d.CreationHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, creditrisk.ErrNotFound
	}
	if err != nil {
		return d, err
	}
	if err = json.Unmarshal(requirements, &d.Requirements); err != nil {
		return d, err
	}
	if err = json.Unmarshal(answers, &d.Answers); err != nil {
		return d, err
	}
	d.Application, err = s.EvidenceApplication(ctx, id)
	return d, err
}
func (s Postgres) UpdateDraft(ctx context.Context, d creditrisk.Draft, expected int, audit creditrisk.Audit, submit bool) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		var status string
		var revision int
		err := tx.QueryRow(ctx, `SELECT a.status,d.revision FROM credit_applications a JOIN credit_application_drafts d ON d.application_id=a.id JOIN credit_application_environments e ON e.application_id=a.id WHERE a.id=$1 AND a.tenant_id=$2 AND e.environment=$3 FOR UPDATE OF a,d`, d.Application.ID, d.Application.TenantID, d.Application.Environment).Scan(&status, &revision)
		if errors.Is(err, pgx.ErrNoRows) {
			return creditrisk.ErrNotFound
		}
		if err != nil {
			return err
		}
		if (status != "draft" && (submit || status != "awaiting_information")) || revision != expected {
			return creditrisk.ErrInvalidState
		}
		raw, err := json.Marshal(d.Answers)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE credit_application_drafts SET answers=$2,revision=revision+1 WHERE application_id=$1`, d.Application.ID, raw); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO credit_application_answer_history(application_id,revision,answers,actor,action) VALUES($1,$2,$3,$4,$5)`, d.Application.ID, expected+1, raw, audit.Actor, audit.Action); err != nil {
			return err
		}
		if submit {
			if _, err = tx.Exec(ctx, `UPDATE credit_applications SET status='pending_review',submitted_at=$2 WHERE id=$1`, d.Application.ID, d.Application.SubmittedAt); err != nil {
				return err
			}
		}
		return appendAudit(ctx, tx, audit)
	})
}
func (s Postgres) DraftEvidence(ctx context.Context, id, tenant, environment string) ([]creditrisk.Evidence, error) {
	// Bound the synchronous verification workload. More evidence requires cleanup
	// or an asynchronous review path, never silent truncation.
	rows, err := s.Pool.Query(ctx, `SELECT e.document_id,e.sha256,e.document_type FROM credit_application_evidence e JOIN credit_applications a ON a.id=e.application_id JOIN credit_application_environments x ON x.application_id=a.id WHERE a.id=$1 AND a.tenant_id=$2 AND x.environment=$3 AND e.tenant_id=$2 AND e.environment=$3 AND e.party_id=a.party_id ORDER BY e.document_id LIMIT 129`, id, tenant, environment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []creditrisk.Evidence{}
	for rows.Next() {
		var e creditrisk.Evidence
		if err = rows.Scan(&e.DocumentID, &e.SHA256, &e.DocumentType); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if len(out) > 128 {
		return nil, creditrisk.ErrEvidenceScope
	}
	return out, rows.Err()
}
