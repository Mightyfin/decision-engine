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
	if _, err = pool.Exec(ctx, `UPDATE credit_applications SET status='offered' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.BindEvidence(ctx, e); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("decided case accepts evidence", err)
	}
}
