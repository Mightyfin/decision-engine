package storage

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"testing"
	"time"
)

func testCancellationGuards(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	s := Postgres{Pool: pool}
	for _, status := range []string{"draft", "pending_review", "awaiting_information", "offered", "accepted", "declined", "cancelled"} {
		a := creditrisk.Application{ID: "cancel-" + status, TenantID: "cancel-tenant", Environment: "sandbox", ProductPolicyID: "p", RelationshipID: "r", Amount: 1000, Currency: "ZMW", TermDays: 30, Purpose: "test", ProductPolicyVersion: 1, RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}, Status: status, SubmittedAt: time.Now()}
		if err := s.CreateApplication(ctx, a, creditrisk.Audit{ApplicationID: a.ID, Actor: "fixture", Action: "created", At: time.Now()}); err != nil {
			t.Fatal(err)
		}
		in := creditrisk.SubmissionIdentity{TenantID: a.TenantID, Environment: "sandbox", CallerApplicationID: "client", Key: "cancel-key-" + status, Hash: "hash-" + status}
		audit := creditrisk.Audit{ApplicationID: a.ID, Actor: "client", Reason: "Applicant withdrew the request", At: time.Now()}
		result, replayed, err := s.CancelApplication(ctx, a, audit, in)
		if status == "accepted" || status == "declined" || status == "cancelled" {
			if !errors.Is(err, creditrisk.ErrInvalidState) {
				t.Fatal(status, err)
			}
			continue
		}
		if err != nil || replayed || result.Status != "cancelled" {
			t.Fatal(status, result, replayed, err)
		}
		var wg sync.WaitGroup
		failures := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, replay, e := s.CancelApplication(ctx, a, audit, in)
				if e == nil && !replay {
					e = errors.New("expected replay")
				}
				failures <- e
			}()
		}
		wg.Wait()
		close(failures)
		for e := range failures {
			if e != nil {
				t.Fatal(e)
			}
		}
		var count int
		if e := pool.QueryRow(ctx, `SELECT count(*) FROM credit_decision_audit WHERE application_id=$1 AND action='cancelled'`, a.ID).Scan(&count); e != nil || count != 1 {
			t.Fatal(count, e)
		}
		if e := pool.QueryRow(ctx, `SELECT count(*) FROM credit_outbox WHERE aggregate_id=$1 AND event_type='credit.application.cancelled'`, a.ID).Scan(&count); e != nil || count != 1 {
			t.Fatal(count, e)
		}
		changed := in
		changed.Hash = "changed"
		if _, _, e := s.CancelApplication(ctx, a, audit, changed); !errors.Is(e, creditrisk.ErrDraftKeyConflict) {
			t.Fatal("changed request", e)
		}
		changed = in
		changed.TenantID = "other"
		if _, _, e := s.CancelApplication(ctx, a, audit, changed); !errors.Is(e, creditrisk.ErrInvalidState) {
			t.Fatal("foreign tenant", e)
		}
	}
}
