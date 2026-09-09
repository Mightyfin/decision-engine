package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Uses the fully migrated disposable schema created by the acceptance test.
func testSubmissionRetries(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	s := Postgres{Pool: pool}
	identity := creditrisk.SubmissionIdentity{TenantID: "submission-tenant", Environment: "sandbox", CallerApplicationID: "client", Key: "submission-key-1234", Hash: "same-request"}
	var wg sync.WaitGroup
	type outcome struct {
		a   creditrisk.Application
		err error
	}
	outcomes := make(chan outcome, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := creditrisk.Application{ID: fmt.Sprintf("submission-%d", i), TenantID: identity.TenantID, Environment: "sandbox", ProductPolicyID: "p", RelationshipID: "r", Amount: 1000, Currency: "ZMW", TermDays: 30, Purpose: "test", ProductPolicyVersion: 1, RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}, Status: "pending_review", SubmittedAt: time.Now()}
			got, err := s.CreateSubmission(ctx, a, creditrisk.Audit{ApplicationID: a.ID, Actor: "client", Action: "submitted", At: time.Now()}, identity)
			outcomes <- outcome{got, err}
		}(i)
	}
	wg.Wait()
	close(outcomes)
	var first string
	for result := range outcomes {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if first == "" {
			first = result.a.ID
		}
		if result.a.ID != first {
			t.Fatal("duplicate applications", first, result.a.ID)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_applications WHERE tenant_id=$1`, identity.TenantID).Scan(&count); err != nil || count != 1 {
		t.Fatal("application count", count, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_decision_audit WHERE application_id=$1`, first).Scan(&count); err != nil || count != 1 {
		t.Fatal("audit count", count, err)
	}
	if err := s.CheckDraftCaller(ctx, first, identity.TenantID, identity.Environment, identity.CallerApplicationID); err != nil {
		t.Fatal("owner denied", err)
	}
	if err := s.CheckDraftCaller(ctx, first, identity.TenantID, identity.Environment, "foreign-client"); !errors.Is(err, creditrisk.ErrNotFound) {
		t.Fatal("direct submission ownership leaked", err)
	}
	scope := creditrisk.ApplicationListScope{TenantID: identity.TenantID, Environment: identity.Environment, CallerApplicationID: "foreign-client"}
	if items, err := s.ListApplications(ctx, scope, creditrisk.ApplicationListFilter{Limit: 10}); err != nil || len(items) != 0 {
		t.Fatal("direct submission list leaked", items, err)
	}
	if items, err := s.TenantTimeline(ctx, scope, first, "", 10); err != nil || len(items) != 0 {
		t.Fatal("direct submission timeline leaked", items, err)
	}
	scope.CallerApplicationID = identity.CallerApplicationID
	if items, err := s.ListApplications(ctx, scope, creditrisk.ApplicationListFilter{Limit: 10}); err != nil || len(items) != 1 {
		t.Fatal("owner discovery failed", items, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_applications SET status='declined' WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	replay, found, err := s.SubmissionReplay(ctx, identity)
	if err != nil || !found || replay.ID != first || replay.Status != "pending_review" {
		t.Fatal("response snapshot lost", replay, found, err)
	}
	other := identity
	other.Hash = "different-request"
	if _, _, err = s.SubmissionReplay(ctx, other); !errors.Is(err, creditrisk.ErrDraftKeyConflict) {
		t.Fatal("changed request", err)
	}
	for _, field := range []string{"tenant", "environment", "caller"} {
		other = identity
		switch field {
		case "tenant":
			other.TenantID = "foreign"
		case "environment":
			other.Environment = "production"
		case "caller":
			other.CallerApplicationID = "other-client"
		}
		if _, found, err = s.SubmissionReplay(ctx, other); err != nil || found {
			t.Fatal("scope leak", field, found, err)
		}
	}
}
