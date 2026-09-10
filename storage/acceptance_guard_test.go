package storage

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
)

func TestAcceptanceTransitionGuard(t *testing.T) {
	dsn := os.Getenv("DECISION_ENGINE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	ctx := context.Background()
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	cfg.MaxConns = 1
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	_, e = p.Exec(ctx, `CREATE TEMP TABLE credit_applications(id text PRIMARY KEY,tenant_id text,status text); CREATE TEMP TABLE credit_offers(application_id text,quote_id text,expires_at timestamptz);INSERT INTO credit_applications VALUES('a','t','offered'); INSERT INTO credit_offers VALUES('a','q',now()+interval '1 day');`)
	if e != nil {
		t.Fatal(e)
	}
	s := Postgres{Pool: p}
	if _, e = p.Exec(ctx, `CREATE TEMP TABLE credit_purchase_restrictions(application_id text PRIMARY KEY)`); e != nil {
		t.Fatal(e)
	}
	a := creditrisk.Application{ID: "a", TenantID: "t", Status: "accepted"}
	// The same conditional transition is used inside the audit + outbox transaction.
	attempt := func(a creditrisk.Application, q string) error {
		return s.withTx(ctx, func(tx pgx.Tx) error { return acceptApplication(ctx, tx, a, q) })
	}
	wrong := a
	wrong.TenantID = "foreign"
	if e = attempt(wrong, "q"); !errors.Is(e, creditrisk.ErrInvalidState) {
		t.Fatal(e)
	}
	if e = attempt(a, "stale-quote"); !errors.Is(e, creditrisk.ErrInvalidState) {
		t.Fatal(e)
	}
	if e = attempt(a, "q"); e != nil {
		t.Fatal(e)
	}
	if e = attempt(a, "q"); !errors.Is(e, creditrisk.ErrInvalidState) {
		t.Fatal("duplicate acceptance", e)
	}
	_, e = p.Exec(ctx, `UPDATE credit_applications SET status='offered'; UPDATE credit_offers SET expires_at=now()-interval '1 second'`)
	if e != nil {
		t.Fatal(e)
	}
	if e = attempt(a, "q"); !errors.Is(e, creditrisk.ErrInvalidState) {
		t.Fatal("expired offer", e)
	}
}
