package storage

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

func testPurchaseRestrictions(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	s := Postgres{Pool: pool}
	a := creditrisk.Application{ID: "purchase_case", TenantID: "purchase_tenant", Environment: "sandbox", ProductPolicyID: "p", RelationshipID: "r", PartyID: "buyer", Currency: "ZMW", Purpose: "purchase test", Amount: 1000, TermDays: 30, ProductPolicyVersion: 1, RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}, Status: "pending_review", SubmittedAt: time.Now()}
	audit := creditrisk.Audit{ApplicationID: a.ID, Actor: "analyst", Action: "submitted", At: time.Now()}
	if err := s.CreateApplication(ctx, a, audit); err != nil {
		t.Fatal(err)
	}
	e := creditrisk.Evidence{ApplicationID: a.ID, TenantID: a.TenantID, Environment: a.Environment, PartyID: a.PartyID, DocumentID: "invoice", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", DocumentType: "invoice", LinkedBy: "applicant"}
	if err := s.BindEvidence(ctx, e); err != nil {
		t.Fatal(err)
	}
	revision := func() string {
		v, err := s.ReviewRevision(ctx, a.TenantID, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	p := creditrisk.PurchaseRestriction{ApplicationID: a.ID, OrderReference: "ORDER-1", SupplierPartyID: "supplier", DestinationWalletID: "supplier-wallet", DocumentID: e.DocumentID, SHA256: e.SHA256, Currency: "ZMW", MaximumAmountMinor: 1000}
	before := revision()
	if err := s.RecordPurchaseRestriction(ctx, p, "foreign", "sandbox", "analyst", "invoice reviewed", before); !errors.Is(err, creditrisk.ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.RecordPurchaseRestriction(ctx, p, a.TenantID, "production", "analyst", "invoice reviewed", before); !errors.Is(err, creditrisk.ErrNotFound) {
		t.Fatal(err)
	}
	wrong := p
	wrong.DocumentID = "unlinked"
	if err := s.RecordPurchaseRestriction(ctx, wrong, a.TenantID, "sandbox", "analyst", "invoice reviewed", before); !errors.Is(err, creditrisk.ErrEvidenceScope) {
		t.Fatal(err)
	}
	wrong = p
	wrong.MaximumAmountMinor++
	if err := s.RecordPurchaseRestriction(ctx, wrong, a.TenantID, "sandbox", "analyst", "invoice reviewed", before); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal(err)
	}
	if err := s.RecordPurchaseRestriction(ctx, p, a.TenantID, "sandbox", "analyst", "invoice reviewed", "0"); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal(err)
	}
	if err := s.RecordPurchaseRestriction(ctx, p, a.TenantID, "sandbox", "analyst", "invoice reviewed", before); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordPurchaseRestriction(ctx, p, a.TenantID, "sandbox", "analyst", "invoice reviewed", before); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal(err)
	}
	got, err := s.PurchaseRestriction(ctx, a.ID, a.TenantID, "sandbox")
	if err != nil || got.RecordedBy != "analyst" || got.DestinationWalletID != p.DestinationWalletID {
		t.Fatal(got, err)
	}
	if _, err = s.PurchaseRestriction(ctx, a.ID, "foreign", "sandbox"); !errors.Is(err, creditrisk.ErrNotFound) {
		t.Fatal(err)
	}
	for _, sql := range []string{`UPDATE credit_purchase_restrictions SET destination_wallet_id='other' WHERE application_id=$1`, `DELETE FROM credit_purchase_restrictions WHERE application_id=$1`} {
		if _, err = pool.Exec(ctx, sql, a.ID); err == nil {
			t.Fatal("restriction history mutated")
		}
	}
	offer := creditrisk.Offer{ApplicationID: a.ID, QuoteID: "purchase_quote", ProductPolicyVersion: 1, PricingPolicyVersion: 1, Principal: 1000, Total: 1000, TermDays: 30, InstallmentCount: 1, RepaymentIntervalDays: 30, AllocationOrder: a.AllocationOrder, PenaltyBasis: "overdue_principal", ExpiresAt: time.Now().Add(time.Hour)}
	a.Status = "offered"
	audit.Action = "offer"
	if err = s.RecordDecision(creditrisk.WithReviewVersion(ctx, revision()), a, &offer, audit); err != nil {
		t.Fatal(err)
	}
	offer, err = s.Offer(ctx, a.ID)
	if err != nil || offer.PurchaseRestriction == nil || offer.PurchaseRestriction.DestinationWalletID != p.DestinationWalletID {
		t.Fatal(offer, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE credit_offers SET purchase_restriction=NULL WHERE application_id=$1`, a.ID); err == nil {
		t.Fatal("offer restriction removed")
	}
	a.Status = "accepted"
	audit.Action = "accepted"
	if err = s.RecordAcceptanceWithEvent(ctx, a, audit, offer); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("restricted purchase entered cash handoff", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE credit_applications SET status='accepted' WHERE id=$1`, a.ID); err == nil {
		t.Fatal("older writer bypassed guard")
	}
}
