package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testTenantEvents(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	s := Postgres{Pool: pool}
	a := creditrisk.Application{ID: "event_case", TenantID: "event_tenant", Environment: "sandbox", ProductPolicyID: "p", RelationshipID: "r", Amount: 1000, Currency: "ZMW", TermDays: 30, Purpose: "test", ProductPolicyVersion: 1, Status: "pending_review", SubmittedAt: time.Now(), AllocationOrder: []string{"penalty", "fees", "interest", "principal"}}
	audit := creditrisk.Audit{ApplicationID: a.ID, Actor: "PRIVATE_ACTOR", Action: "submitted", Reason: "PRIVATE_REASON", At: time.Now()}
	a.RepaymentIntervalDays = 30
	in := creditrisk.SubmissionIdentity{TenantID: a.TenantID, Environment: "sandbox", CallerApplicationID: "app_owner", Key: "event-test-key", Hash: strings.Repeat("a", 64)}
	if _, err := s.CreateSubmission(ctx, a, audit, in); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSubmission(ctx, a, audit, in); err != nil {
		t.Fatal(err)
	}
	var count int
	var payload string
	if err := pool.QueryRow(ctx, `SELECT count(*),min(payload::text) FROM credit_outbox WHERE aggregate_id=$1 AND event_type='credit.application.status_changed'`, a.ID).Scan(&count, &payload); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if strings.Contains(payload, "PRIVATE") || !strings.Contains(payload, "app_owner") {
		t.Fatal("unsafe or unscoped event", payload)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_applications SET status='offered' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	offer := creditrisk.Offer{ApplicationID: a.ID, QuoteID: "ownership_quote", ProductPolicyVersion: 1, PricingPolicyVersion: 1, Principal: 1000, Total: 1000, TermDays: 30, ExpiresAt: time.Now().Add(time.Hour)}
	offer.AllocationOrder = []string{"penalty", "fees", "interest", "principal"}
	offer.InstallmentCount = 1
	offer.RepaymentIntervalDays = 30
	offer.PenaltyBasis = "overdue_principal"
	if err := s.SaveOffer(ctx, offer); err != nil {
		t.Fatal(err)
	}
	a.Status = "accepted"
	audit.Action = "accepted"
	if err := s.RecordAcceptanceWithEvent(ctx, a, audit, offer); err != nil {
		t.Fatal(err)
	}
	var caller string
	if err := pool.QueryRow(ctx, `SELECT payload->>'caller_application_id' FROM credit_outbox WHERE aggregate_id=$1 AND event_type='credit.offer.accepted'`, a.ID).Scan(&caller); err != nil || caller != "app_owner" {
		t.Fatal("handoff lost original owner", caller, err)
	}
	a.Status = "pending_review"
	audit.Action = "submitted"
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_outbox ADD CONSTRAINT reject_status_test CHECK(event_type<>'credit.application.status_changed') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	a.ID = "event_rollback"
	audit.ApplicationID = a.ID
	in.Key = "event-rollback-key"
	if _, err := s.CreateSubmission(ctx, a, audit, in); err == nil {
		t.Fatal("expected outbox failure")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_applications WHERE id=$1`, a.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial application", count, err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_outbox DROP CONSTRAINT reject_status_test`); err != nil {
		t.Fatal(err)
	}
}
