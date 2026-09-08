package storage

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5"
)

func (s Postgres) EvidenceApplication(ctx context.Context, id string) (creditrisk.Application, error) {
	a, err := s.Application(ctx, id)
	if err != nil {
		return a, err
	}
	err = s.Pool.QueryRow(ctx, `SELECT environment FROM credit_application_environments WHERE application_id=$1`, id).Scan(&a.Environment)
	if errors.Is(err, pgx.ErrNoRows) {
		// The service checks tenant ownership before reporting missing provenance.
		return a, nil
	}
	return a, err
}
func (s Postgres) BindEvidence(ctx context.Context, e creditrisk.Evidence) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT a.status FROM credit_applications a JOIN credit_application_environments x ON x.application_id=a.id WHERE a.id=$1 AND a.tenant_id=$2 AND a.party_id=$3 AND x.environment=$4 FOR UPDATE OF a`, e.ApplicationID, e.TenantID, e.PartyID, e.Environment).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return creditrisk.ErrNotFound
		}
		if err != nil {
			return err
		}
		if status != "pending_review" {
			return creditrisk.ErrInvalidState
		}
		tag, err := tx.Exec(ctx, `INSERT INTO credit_application_evidence(application_id,document_id,sha256,tenant_id,environment,party_id,document_type,linked_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(application_id,document_id,sha256) DO NOTHING`, e.ApplicationID, e.DocumentID, e.SHA256, e.TenantID, e.Environment, e.PartyID, e.DocumentType, e.LinkedBy)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		return appendAudit(ctx, tx, creditrisk.Audit{ApplicationID: e.ApplicationID, Actor: e.LinkedBy, Action: "evidence_linked", Reason: "Document " + e.DocumentID + " version " + e.SHA256})
	})
}
