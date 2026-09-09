package httpapi

import (
	"context"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
)

// In-memory adapter for HTTP shape tests. Durability/concurrency is tested
// against PostgreSQL in storage, never inferred from this fake.
func (s *testStore) SubmissionReplay(context.Context, creditrisk.SubmissionIdentity) (creditrisk.Application, bool, error) {
	return creditrisk.Application{}, false, nil
}
func (s *testStore) CreateSubmission(ctx context.Context, a creditrisk.Application, audit creditrisk.Audit, _ creditrisk.SubmissionIdentity) (creditrisk.Application, error) {
	if err := s.SaveApplication(ctx, a); err != nil {
		return a, err
	}
	return a, s.AppendAudit(ctx, audit)
}
