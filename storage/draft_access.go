package storage

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5"
)

// Legacy applications keep their existing policy; new workload-created drafts
// and their submitted cases retain the creating application's access boundary.
func (s Postgres) CheckDraftCaller(ctx context.Context, id, tenant, environment, caller string) error {
	var owner, storedTenant, storedEnvironment string
	err := s.Pool.QueryRow(ctx, `SELECT d.caller_application_id,a.tenant_id,e.environment FROM credit_application_drafts d JOIN credit_applications a ON a.id=d.application_id JOIN credit_application_environments e ON e.application_id=a.id WHERE a.id=$1`, id).Scan(&owner, &storedTenant, &storedEnvironment)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if owner != "" && (owner != caller || storedTenant != tenant || storedEnvironment != environment) {
		return creditrisk.ErrNotFound
	}
	return nil
}
