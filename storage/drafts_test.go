package storage

import (
	"context"
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/migrations"
	"github.com/Mightyfin/decision-engine/product"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type draftProducts struct{ p product.Policy }

func (p *draftProducts) Policy(context.Context, string, string) (product.Policy, error) {
	return p.p, nil
}

type draftVerifier struct{ fail bool }

func (v draftVerifier) VerifyEvidence(context.Context, creditrisk.Application, string, string) (string, error) {
	if v.fail {
		return "", errors.New("scan unavailable")
	}
	return "IDENTITY", nil
}

func TestDraftLifecycle(t *testing.T) {
	dsn := os.Getenv("DECISION_DRAFT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable database required")
	}
	ctx := context.Background()
	ctx = creditrisk.WithDraftCreation(ctx, "tenant-app-a", "request-hash-a")
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
	store := Postgres{Pool: pool}
	p := &draftProducts{product.Policy{ID: "prd_draft", TenantID: "draft_tenant", Version: 1, Active: true, Currency: "ZMW", MinimumAmount: 100, MaximumAmount: 100000, MinimumTermDays: 1, MaximumTermDays: 365, RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}, RequiredDocumentTypes: []string{"IDENTITY"}, ApplicationRequirements: map[string]product.Requirements{"network_participant": {Fields: []product.Field{{Key: "income", Type: "number", Required: true}}}}}}
	s := creditrisk.Service{Store: store, Products: product.Service{Store: p}, Clock: func() time.Time { return time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC) }}
	a := creditrisk.Application{ID: "draft_lifecycle", TenantID: "draft_tenant", Environment: "sandbox", ProductPolicyID: "prd_draft", RelationshipID: "rel", PartyID: "party", ApplicantRole: "network_participant", Currency: "ZMW", Purpose: "stock", Amount: 1000, TermDays: 30}
	if _, err = s.Submit(ctx, a, "tenant"); !errors.Is(err, creditrisk.ErrDraftRequired) {
		t.Fatal("direct bypass", err)
	}
	d, err := s.CreateDraft(ctx, a, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		replayed, err := s.CreateDraft(ctx, a, "tenant")
		if err != nil || replayed.Application.ID != a.ID || replayed.Revision != 1 {
			t.Fatal("draft replay", replayed, err)
		}
	}
	if _, err = s.CreateDraft(creditrisk.WithDraftCreation(ctx, "tenant-app-a", "changed-input"), a, "tenant"); !errors.Is(err, creditrisk.ErrDraftKeyConflict) {
		t.Fatal("key conflict", err)
	}
	if err = store.CheckDraftCaller(ctx, a.ID, a.TenantID, a.Environment, "tenant-app-a"); err != nil {
		t.Fatal("owner access", err)
	}
	if err = store.CheckDraftCaller(ctx, a.ID, a.TenantID, a.Environment, "tenant-app-b"); !errors.Is(err, creditrisk.ErrNotFound) {
		t.Fatal("other application access", err)
	}
	concurrent := a
	concurrent.ID = "draft_concurrent_create"
	var group sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() { defer group.Done(); _, err := s.CreateDraft(ctx, concurrent, "tenant"); failures <- err }()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal("concurrent retry", err)
		}
	}
	var creations int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM credit_decision_audit WHERE application_id=$1 AND action='draft_created'`, concurrent.ID).Scan(&creations); err != nil || creations != 1 {
		t.Fatal("duplicate draft creation audit", creations, err)
	}
	if q, err := store.ReviewQueue(ctx, a.TenantID, 20); err != nil || len(q) != 0 {
		t.Fatal("draft in queue", q, err)
	}
	for _, scope := range [][2]string{{"other", "sandbox"}, {a.TenantID, "production"}} {
		if _, err = store.GetDraft(ctx, a.ID, scope[0], scope[1]); !errors.Is(err, creditrisk.ErrNotFound) {
			t.Fatal("scope leak", err)
		}
	}
	if _, err = s.SubmitDraft(ctx, a.ID, a.TenantID, a.Environment, "tenant", 1, draftVerifier{}); err == nil {
		t.Fatal("incomplete submitted")
	}
	d, err = s.SaveDraft(ctx, a.ID, a.TenantID, a.Environment, "tenant", d.Revision, map[string]json.RawMessage{"income": json.RawMessage(`1000`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveDraft(ctx, a.ID, a.TenantID, a.Environment, "tenant", 1, nil); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("stale edit", err)
	}
	e := creditrisk.Evidence{ApplicationID: a.ID, TenantID: a.TenantID, Environment: a.Environment, PartyID: a.PartyID, DocumentID: "doc", SHA256: strings.Repeat("a", 64), DocumentType: "IDENTITY", LinkedBy: "tenant"}
	if err = (creditrisk.EvidenceService{Store: store, Verifier: draftVerifier{}}).Bind(ctx, a.ID, a.TenantID, a.Environment, "tenant", e.DocumentID, e.SHA256); err != nil {
		t.Fatal("draft evidence", err)
	}
	if _, err = s.SubmitDraft(ctx, a.ID, a.TenantID, a.Environment, "tenant", d.Revision, draftVerifier{fail: true}); err == nil {
		t.Fatal("unavailable verifier accepted")
	}
	p.p.Version = 2
	if _, err = s.SubmitDraft(ctx, a.ID, a.TenantID, a.Environment, "tenant", d.Revision, draftVerifier{}); !errors.Is(err, creditrisk.ErrProductChanged) {
		t.Fatal("stale product", err)
	}
	p.p.Version = 1
	d, err = s.SubmitDraft(ctx, a.ID, a.TenantID, a.Environment, "tenant", d.Revision, draftVerifier{})
	if err != nil || d.Application.Status != "pending_review" {
		t.Fatal(d, err)
	}
	questionnaire, err := store.ReviewQuestionnaire(ctx, a.ID, a.TenantID, a.Environment)
	if err != nil || questionnaire == nil || questionnaire.ProductVersion != 1 || string(questionnaire.Answers["income"]) != "1000" {
		t.Fatal("review snapshot", questionnaire, err)
	}
	for _, scope := range [][2]string{{"other", "sandbox"}, {a.TenantID, "production"}} {
		if _, err = store.ReviewQuestionnaire(ctx, a.ID, scope[0], scope[1]); !errors.Is(err, creditrisk.ErrNotFound) {
			t.Fatal("questionnaire scope", err)
		}
	}
	legacy := d.Application
	legacy.ID = "legacy_questionnaire"
	legacy.Status = "pending_review"
	if err = store.CreateApplication(ctx, legacy, creditrisk.Audit{ApplicationID: legacy.ID, Actor: "test", Action: "submitted", Reason: "legacy test"}); err != nil {
		t.Fatal(err)
	}
	if q, err := store.ReviewQuestionnaire(ctx, legacy.ID, legacy.TenantID, legacy.Environment); err != nil || q != nil {
		t.Fatal("legacy questionnaire", q, err)
	}
	if _, err = s.SubmitDraft(ctx, a.ID, a.TenantID, a.Environment, "tenant", d.Revision, draftVerifier{}); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("double submit", err)
	}
	if _, err = s.SaveDraft(ctx, a.ID, a.TenantID, a.Environment, "tenant", d.Revision, nil); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("editing submitted case", err)
	}
	if q, err := store.ReviewQueue(ctx, a.TenantID, 20); err != nil || len(q) != 2 {
		t.Fatal("submitted queue", q, err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM credit_decision_audit WHERE application_id=$1 AND action='submitted'`, a.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("submission audit", count, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM credit_outbox WHERE aggregate_id=$1`, a.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("unexpected funding handoff", count, err)
	}
	reviewVersion, err := store.ReviewRevision(ctx, a.TenantID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ChangeInformationState(ctx, a.ID, a.TenantID, a.Environment, "analyst", "Please correct the income response", reviewVersion, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveDraft(ctx, a.ID, a.TenantID, a.Environment, "tenant", d.Revision, nil); err == nil {
		t.Fatal("required answer removed during information request")
	}
	updated, err := s.SaveDraft(ctx, a.ID, a.TenantID, a.Environment, "tenant", d.Revision, map[string]json.RawMessage{"income": json.RawMessage(`1500`)})
	if err != nil || updated.Application.Status != "awaiting_information" {
		t.Fatal("correction workflow", updated, err)
	}
	if q, err := store.ReviewQuestionnaire(ctx, a.ID, a.TenantID, a.Environment); err != nil || string(q.Answers["income"]) != "1500" {
		t.Fatal("corrected review snapshot", q, err)
	}
	var original string
	if err = pool.QueryRow(ctx, `SELECT answers->>'income' FROM credit_application_answer_history WHERE application_id=$1 AND revision=2`, a.ID).Scan(&original); err != nil || original != "1000" {
		t.Fatal("original answers not retained", original, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE credit_application_answer_history SET answers='{}' WHERE application_id=$1`, a.ID); err == nil {
		t.Fatal("answer history is mutable")
	}
}
