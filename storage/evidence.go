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

func (s Postgres) HasEvidence(ctx context.Context, id, tenant, environment, document, digest string) (bool, error) {
	var found bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credit_application_evidence e JOIN credit_applications a ON a.id=e.application_id JOIN credit_application_environments x ON x.application_id=a.id WHERE e.application_id=$1 AND e.tenant_id=$2 AND a.tenant_id=$2 AND e.environment=$3 AND x.environment=$3 AND e.party_id=a.party_id AND e.document_id=$4 AND e.sha256=$5)`, id, tenant, environment, document, digest).Scan(&found)
	return found, err
}

func (s Postgres) EvidencePage(ctx context.Context, id, tenant, environment, afterDoc, afterHash string, limit int) ([]creditrisk.Evidence, error) {
	if tenant == "" || environment == "" {
		return nil, creditrisk.ErrEvidenceScope
	}
	if limit < 1 || limit > 51 {
		limit = 51
	}
	rows, err := s.Pool.Query(ctx, `SELECT e.application_id,e.document_id,e.sha256,e.tenant_id,e.environment,e.party_id,e.document_type,e.linked_by,e.linked_at FROM credit_application_evidence e JOIN credit_applications a ON a.id=e.application_id JOIN credit_application_environments x ON x.application_id=a.id WHERE e.application_id=$1 AND e.tenant_id=$2 AND a.tenant_id=$2 AND e.environment=$3 AND x.environment=$3 AND e.party_id=a.party_id AND ($4='' OR (e.document_id,e.sha256)>($4,$5)) ORDER BY e.document_id,e.sha256 LIMIT $6`, id, tenant, environment, afterDoc, afterHash, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []creditrisk.Evidence{}
	for rows.Next() {
		var e creditrisk.Evidence
		if err = rows.Scan(&e.ApplicationID, &e.DocumentID, &e.SHA256, &e.TenantID, &e.Environment, &e.PartyID, &e.DocumentType, &e.LinkedBy, &e.LinkedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
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
		if status != "pending_review" && status != "awaiting_information" {
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
