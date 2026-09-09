package storage

import (
	"context"
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/product"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"testing"
	"time"
)

func testCommercialReview(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	s := Postgres{Pool: pool}
	a := creditrisk.Application{ID: "commercial-app", TenantID: "commercial-tenant", Environment: "sandbox", ProductPolicyID: "p", RelationshipID: "r", PartyID: "party", ApplicantRole: "network_participant", Amount: 1000, Currency: "ZMW", TermDays: 30, Purpose: "stock", ProductPolicyVersion: 1, RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}, Status: "draft"}
	d := creditrisk.Draft{Application: a, CallerApplicationID: "client", Requirements: product.Requirements{CommercialReviewRequired: true}, Answers: map[string]json.RawMessage{}, Revision: 1}
	if err := s.CreateDraft(ctx, d, creditrisk.Audit{ApplicationID: a.ID, Actor: "maker", Action: "draft_created", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	identity := creditrisk.SubmissionIdentity{TenantID: a.TenantID, Environment: "sandbox", CallerApplicationID: "client", Key: "commercial-key-1234", Hash: "review-body"}
	current, err := s.CommercialReview(ctx, a.ID, identity)
	if err != nil || current.Decision != "pending" || len(current.SnapshotHash) != 64 {
		t.Fatal(current, err)
	}
	audit := creditrisk.Audit{ApplicationID: a.ID, Actor: "service-reviewer", Reason: "Tenant confirms this commercial request", At: time.Now()}
	if err = s.UpdateDraft(ctx, d, 1, audit, true); !errors.Is(err, creditrisk.ErrCommercialReviewRequired) {
		t.Fatal("submission bypassed tenant review", err)
	}
	review := current
	review.Decision = "approved"
	review.Reference = "tenant-review-1"
	stale := review
	stale.SnapshotHash = "old-snapshot"
	if _, _, err = s.RecordCommercialReview(ctx, stale, audit, identity); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("stale review accepted", err)
	}
	foreign := identity
	foreign.CallerApplicationID = "other-client"
	if _, _, err = s.RecordCommercialReview(ctx, review, audit, foreign); !errors.Is(err, creditrisk.ErrNotFound) {
		t.Fatal("foreign client reviewed", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, e := s.RecordCommercialReview(ctx, review, audit, identity); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM credit_decision_audit WHERE application_id=$1 AND action='commercial_approved'`, a.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate audit", count, err)
	}
	if _, replayed, e := s.RecordCommercialReview(ctx, review, audit, identity); e != nil || !replayed {
		t.Fatal("review replay lost", e)
	}
	changed := identity
	changed.Hash = "changed-body"
	if _, _, e := s.RecordCommercialReview(ctx, review, audit, changed); !errors.Is(e, creditrisk.ErrDraftKeyConflict) {
		t.Fatal("changed key body accepted", e)
	}
	// Editing the draft invalidates the earlier approval even if it was approved.
	d.Revision = 2
	if err = s.UpdateDraft(ctx, d, 1, audit, false); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateDraft(ctx, d, 2, audit, true); !errors.Is(err, creditrisk.ErrCommercialReviewRequired) {
		t.Fatal("old revision approval reused", err)
	}
	current, err = s.CommercialReview(ctx, a.ID, identity)
	if err != nil || current.Decision != "pending" {
		t.Fatal(current, err)
	}
	current.Decision = "approved"
	current.Reference = "tenant-review-2"
	identity.Key = "commercial-key-5678"
	identity.Hash = "review-body-2"
	if _, _, err = s.RecordCommercialReview(ctx, current, audit, identity); err != nil {
		t.Fatal(err)
	}
	// New linked evidence invalidates approval of the previously reviewed set.
	if _, err = pool.Exec(ctx, `INSERT INTO credit_application_evidence(application_id,document_id,sha256,tenant_id,environment,party_id,document_type,linked_by) VALUES($1,'document',repeat('a',64),$2,'sandbox','party','BANK_STATEMENT','client')`, a.ID, a.TenantID); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateDraft(ctx, d, 2, audit, true); !errors.Is(err, creditrisk.ErrCommercialReviewRequired) {
		t.Fatal("unreviewed evidence bypass", err)
	}
	current, err = s.CommercialReview(ctx, a.ID, identity)
	if err != nil || current.Decision != "pending" {
		t.Fatal(current, err)
	}
	current.Decision = "approved"
	current.Reference = "tenant-review-3"
	identity.Key = "commercial-key-9012"
	identity.Hash = "review-body-3"
	if _, _, err = s.RecordCommercialReview(ctx, current, audit, identity); err != nil {
		t.Fatal(err)
	}
	d.Application.SubmittedAt = time.Now()
	if err = s.UpdateDraft(ctx, d, 2, audit, true); err != nil {
		t.Fatal("reviewed submission failed", err)
	}
	after, err := s.EvidenceApplication(ctx, a.ID)
	if err != nil || after.Status != "pending_review" {
		t.Fatal("commercial approval did not preserve lender review", after, err)
	}
	current, err = s.CommercialReview(ctx, a.ID, identity)
	if err != nil || current.Decision != "approved" {
		t.Fatal("submitted approval visibility lost", current, err)
	}
	version, err := s.ReviewRevision(ctx, a.TenantID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeInformationState(ctx, a.ID, a.TenantID, a.Environment, "analyst", "Please correct the supporting information", version, false); err != nil {
		t.Fatal(err)
	}
	version, err = s.ReviewRevision(ctx, a.TenantID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeInformationState(ctx, a.ID, a.TenantID, a.Environment, "client", "Supporting information has been checked", version, true); !errors.Is(err, creditrisk.ErrCommercialReviewRequired) {
		t.Fatal("RFI bypassed renewed commercial approval", err)
	}
	current, err = s.CommercialReview(ctx, a.ID, identity)
	if err != nil || current.Decision != "pending" {
		t.Fatal(current, err)
	}
	current.Decision = "rejected"
	current.Reference = "tenant-review-rejected"
	identity.Key = "commercial-key-rejection"
	identity.Hash = "rejection-body"
	if _, _, err = s.RecordCommercialReview(ctx, current, audit, identity); err != nil {
		t.Fatal(err)
	}
	version, err = s.ReviewRevision(ctx, a.TenantID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeInformationState(ctx, a.ID, a.TenantID, a.Environment, "client", "Attempt to resubmit rejected business case", version, true); !errors.Is(err, creditrisk.ErrCommercialReviewRequired) {
		t.Fatal("rejected commercial review progressed", err)
	}
}
