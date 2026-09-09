package storage

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5"
)

// Legacy records without caller provenance keep their existing policy. New
// drafts and direct submissions retain the creating application's boundary.
func (s Postgres) CheckDraftCaller(ctx context.Context, id, tenant, environment, caller string) error {
	var owner, submissionOwner, storedTenant, storedEnvironment string
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(d.caller_application_id,''),e.caller_application_id,a.tenant_id,e.environment FROM credit_applications a JOIN credit_application_environments e ON e.application_id=a.id LEFT JOIN credit_application_drafts d ON d.application_id=a.id WHERE a.id=$1`, id).Scan(&owner, &submissionOwner, &storedTenant, &storedEnvironment)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if storedTenant != tenant || storedEnvironment != environment || (owner != "" && owner != caller) || (submissionOwner != "" && submissionOwner != caller) {
		return creditrisk.ErrNotFound
	}
	return nil
}
