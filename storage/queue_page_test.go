package storage

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
)

func TestQueueCursorIsolationAndDecidedAnchor(t *testing.T) {
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
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	_, e = pool.Exec(ctx, `CREATE TEMP TABLE credit_applications(id text,tenant_id text,product_policy_id text DEFAULT 'product',relationship_id text DEFAULT 'relationship',party_id text,applicant_role text,wallet_id text,origin text,currency text DEFAULT 'ZMW',purpose text DEFAULT 'stock',status text DEFAULT 'pending_review',amount bigint DEFAULT 500000,term_days int DEFAULT 30,product_policy_version int DEFAULT 1,repayment_interval_days int DEFAULT 30,grace_days int DEFAULT 3,allocation_order text[] DEFAULT ARRAY['principal'],submitted_at timestamptz DEFAULT '2026-09-01'); INSERT INTO credit_applications(id,tenant_id) VALUES('a','tenant-a'),('b','tenant-a'),('c','tenant-b');`)
	if e != nil {
		t.Fatal(e)
	}
	s := Postgres{Pool: pool}
	_, e = pool.Exec(ctx, `CREATE TEMP TABLE credit_decision_audit(id bigserial,application_id text,actor text,action text,reason text,created_at timestamptz DEFAULT now()); INSERT INTO credit_decision_audit(application_id,actor,action,reason) VALUES('a','analyst','offered','Reviewed evidence'),('c','other','declined','Other tenant')`)
	if e != nil {
		t.Fatal(e)
	}
	history, e := s.ReviewHistory(ctx, "tenant-a", "a")
	if e != nil || len(history) != 1 || history[0].Actor != "analyst" {
		t.Fatal(history, e)
	}
	history, e = s.ReviewHistory(ctx, "tenant-b", "a")
	if e != nil || len(history) != 0 {
		t.Fatal("cross-tenant history", history, e)
	}
	rows, e := s.ReviewQueuePage(ctx, "tenant-a", 1, "")
	if e != nil || len(rows) != 1 || rows[0].ID != "a" {
		t.Fatal(rows, e)
	}
	_, e = pool.Exec(ctx, `UPDATE credit_applications SET status='offered' WHERE id='a'`)
	if e != nil {
		t.Fatal(e)
	}
	rows, e = s.ReviewQueuePage(ctx, "tenant-a", 2, "a")
	if e != nil || len(rows) != 1 || rows[0].ID != "b" {
		t.Fatal(rows, e)
	}
	rows, e = s.ReviewQueuePage(ctx, "tenant-b", 2, "a")
	if e != nil || len(rows) != 0 {
		t.Fatal("cross-tenant cursor", rows, e)
	}
}
