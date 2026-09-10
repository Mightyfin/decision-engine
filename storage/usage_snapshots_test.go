package storage

import (
	"context"
	"encoding/json"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

func testUsageSnapshots(t *testing.T, pool *pgxpool.Pool) {
	testPurchaseRestrictions(t, pool)
	ctx := context.Background()
	s := Postgres{Pool: pool}
	usage := json.RawMessage(`{"funding_mode":"borrower_cash","destination_rule":"borrower_wallet","allow_partial_use":false,"usage_expiry_days":0,"repayment_restoration":"none"}`)
	a := creditrisk.Application{ID: "usage_case", TenantID: "usage_tenant", Environment: "sandbox", ProductPolicyID: "p", RelationshipID: "r", Currency: "ZMW", Purpose: "test", Amount: 1000, TermDays: 30, ProductPolicyVersion: 1, RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}, Status: "pending_review", SubmittedAt: time.Now(), UsageTerms: usage}
	audit := creditrisk.Audit{ApplicationID: a.ID, Actor: "staff", Action: "submitted", At: time.Now()}
	if err := s.CreateApplication(ctx, a, audit); err != nil {
		t.Fatal(err)
	}
	// Synthetic verified version; production evidence binding requires the
	// document service's ownership, version and scan checks first.
	if _, err := pool.Exec(ctx, `UPDATE credit_applications SET party_id='snapshot-party' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	a.PartyID = "snapshot-party"
	evidence := creditrisk.Evidence{ApplicationID: a.ID, TenantID: a.TenantID, Environment: "sandbox", PartyID: a.PartyID, DocumentID: "snapshot-invoice", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", DocumentType: "invoice", LinkedBy: "uploader"}
	if err := s.BindEvidence(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_applications SET usage_terms=NULL WHERE id=$1`, a.ID); err == nil {
		t.Fatal("application snapshot mutable")
	}
	got, err := s.Application(ctx, a.ID)
	if err != nil || len(got.UsageTerms) == 0 {
		t.Fatal("application read lost snapshot", err)
	}
	rows, err := s.ListApplications(ctx, creditrisk.ApplicationListScope{TenantID: a.TenantID, Environment: "sandbox"}, creditrisk.ApplicationListFilter{Limit: 20})
	if err != nil || len(rows) != 1 || len(rows[0].UsageTerms) == 0 {
		t.Fatal("list lost snapshot", err)
	}
	o := creditrisk.Offer{ApplicationID: a.ID, QuoteID: "usage_quote", ProductPolicyVersion: 1, PricingPolicyVersion: 1, Principal: 1000, Total: 1000, TermDays: 30, InstallmentCount: 1, RepaymentIntervalDays: 30, AllocationOrder: a.AllocationOrder, PenaltyBasis: "overdue_principal", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.SaveOffer(ctx, o); err == nil {
		t.Fatal("offer omitted application usage")
	}
	o.UsageTerms = usage
	o.EvidenceSnapshot = []creditrisk.Evidence{{DocumentID: "caller-injected"}}
	a.Status = "offered"
	audit.Action = "offer"
	var version string
	if err := pool.QueryRow(ctx, `SELECT max(id)::text FROM credit_decision_audit WHERE application_id=$1`, a.ID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDecision(creditrisk.WithReviewVersion(ctx, version), a, &o, audit); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_offers SET usage_terms=NULL WHERE application_id=$1`, a.ID); err == nil {
		t.Fatal("offer snapshot mutable")
	}
	o, err = s.Offer(ctx, a.ID)
	if err != nil || len(o.UsageTerms) == 0 {
		t.Fatal("offer read lost snapshot", err)
	}
	if len(o.EvidenceSnapshot) != 1 || o.EvidenceSnapshot[0].DocumentID != evidence.DocumentID || o.EvidenceSnapshot[0].SHA256 != evidence.SHA256 {
		t.Fatal("offer lost authoritative evidence", o.EvidenceSnapshot)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_offers SET evidence_snapshot='[]'::jsonb WHERE application_id=$1`, a.ID); err == nil {
		t.Fatal("offer evidence changed")
	}
	evidence.DocumentID = "late-invoice"
	if err := s.BindEvidence(ctx, evidence); err == nil {
		t.Fatal("evidence linked after decision")
	}
	o.UsageTerms = nil // The persisted offer, not the in-memory caller, is the handoff authority.
	a.Status = "accepted"
	audit.Action = "accepted"
	if err := s.RecordAcceptanceWithEvent(ctx, a, audit, o); err != nil {
		t.Fatal(err)
	}
	var matches bool
	if err := pool.QueryRow(ctx, `SELECT payload->'usage_terms'=$2::jsonb FROM credit_outbox WHERE aggregate_id=$1 AND event_type='credit.offer.accepted'`, a.ID, usage).Scan(&matches); err != nil || !matches {
		t.Fatal("handoff lost stored usage", err)
	}
}
