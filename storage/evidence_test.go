package storage

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEvidencePersistence(t *testing.T) {
	dsn := os.Getenv("DECISION_EVIDENCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable database required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = migrations.Up(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Up(ctx, pool); err != nil {
		t.Fatal("migration replay", err)
	}
	s := Postgres{Pool: pool}
	a := creditrisk.Application{ID: "evidence-test", TenantID: "tenant", Environment: "sandbox", PartyID: "party", ApplicantRole: "network_participant", RelationshipID: "rel", ProductPolicyID: "external", ProductPolicyVersion: 1, Currency: "ZMW", Amount: 10000, TermDays: 30, RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}, Purpose: "test", Status: "pending_review", SubmittedAt: time.Now()}
	if err = s.CreateApplication(ctx, a, creditrisk.Audit{ApplicationID: a.ID, Actor: "actor", Action: "submitted", Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	saved, err := s.EvidenceApplication(ctx, a.ID)
	if err != nil || saved.Environment != "sandbox" {
		t.Fatal(saved, err)
	}
	e := creditrisk.Evidence{ApplicationID: a.ID, TenantID: a.TenantID, Environment: a.Environment, PartyID: a.PartyID, DocumentID: "doc", SHA256: strings.Repeat("a", 64), DocumentType: "BANK_STATEMENT", LinkedBy: "actor"}
	for i := 0; i < 2; i++ {
		if err = s.BindEvidence(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	rows, err := s.EvidencePage(ctx, a.ID, a.TenantID, a.Environment, "", "", 51)
	for _, scope := range []struct {
		tenant, environment, hash string
		want                      bool
	}{{a.TenantID, a.Environment, e.SHA256, true}, {"foreign", a.Environment, e.SHA256, false}, {a.TenantID, "production", e.SHA256, false}, {a.TenantID, a.Environment, strings.Repeat("b", 64), false}} {
		found, checkErr := s.HasEvidence(ctx, a.ID, scope.tenant, scope.environment, e.DocumentID, scope.hash)
		if checkErr != nil || found != scope.want {
			t.Fatalf("case document binding: %v %v", found, checkErr)
		}
	}
	if err != nil || len(rows) != 1 {
		t.Fatal("evidence list", rows, err)
	}
	rows, err = s.EvidencePage(ctx, a.ID, "foreign", a.Environment, "", "", 51)
	if err != nil || len(rows) != 0 {
		t.Fatal("cross-tenant evidence list", rows, err)
	}
	rows, err = s.EvidencePage(ctx, a.ID, a.TenantID, "production", "", "", 51)
	if err != nil || len(rows) != 0 {
		t.Fatal("cross-environment evidence list", rows, err)
	}
	rows, err = s.EvidencePage(ctx, a.ID, a.TenantID, a.Environment, e.DocumentID, e.SHA256, 51)
	if err != nil || len(rows) != 0 {
		t.Fatal("evidence cursor", rows, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM credit_decision_audit WHERE application_id=$1 AND action='evidence_linked'`, a.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate audit", count, err)
	}
	foreign := e
	foreign.TenantID = "foreign"
	if err = s.BindEvidence(ctx, foreign); !errors.Is(err, creditrisk.ErrNotFound) {
		t.Fatal("foreign tenant", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE credit_application_evidence SET sha256=repeat('b',64)`); err == nil {
		t.Fatal("mutable evidence")
	}
	if _, err = pool.Exec(ctx, `UPDATE credit_application_environments SET environment='production'`); err == nil {
		t.Fatal("mutable environment")
	}
	version, err := s.ReviewRevision(ctx, a.TenantID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeInformationState(ctx, a.ID, "foreign", a.Environment, "analyst", "Please provide bank statements", version, false); !errors.Is(err, creditrisk.ErrNotFound) {
		t.Fatal("foreign request", err)
	}
	if err = s.ChangeInformationState(ctx, a.ID, a.TenantID, a.Environment, "analyst", "Please provide bank statements", version, false); err != nil {
		t.Fatal(err)
	}
	a.Status = "declined"
	if err = s.RecordDecision(creditrisk.WithReviewVersion(ctx, version), a, nil, creditrisk.Audit{ApplicationID: a.ID, Actor: "analyst", Action: "decline", Reason: "test decision"}); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("decision while awaiting information", err)
	}
	e.DocumentID = "additional-document"
	if err = s.BindEvidence(ctx, e); err != nil {
		t.Fatal("evidence while awaiting information", err)
	}
	if err = s.ChangeInformationState(ctx, a.ID, a.TenantID, a.Environment, "tenant", "Statements have been attached", version, true); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("stale resubmission", err)
	}
	latest, err := s.ReviewRevision(ctx, a.TenantID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeInformationState(ctx, a.ID, a.TenantID, a.Environment, "tenant", "Statements have been attached", latest, true); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordDecision(creditrisk.WithReviewVersion(ctx, version), a, nil, creditrisk.Audit{ApplicationID: a.ID, Actor: "analyst", Action: "decline", Reason: "test decision"}); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("stale decision after resubmission", err)
	}
	latest, err = s.ReviewRevision(ctx, a.TenantID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordDecision(creditrisk.WithReviewVersion(ctx, latest), a, nil, creditrisk.Audit{ApplicationID: a.ID, Actor: "analyst", Action: "decline", Reason: "Reviewed updated case"}); err != nil {
		t.Fatal(err)
	}
	if err = s.BindEvidence(ctx, e); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("decided case accepts evidence", err)
	}
}
