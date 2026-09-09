package storage

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
)

func TestApplicationListIsolationAndPagination(t *testing.T) {
	dsn := os.Getenv("DECISION_ENGINE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TEMP TABLE credit_applications(id text PRIMARY KEY,tenant_id text,product_policy_id text DEFAULT 'product',relationship_id text DEFAULT 'relationship',party_id text,applicant_role text,wallet_id text,origin text,currency text DEFAULT 'ZMW',purpose text DEFAULT 'stock',status text DEFAULT 'pending_review',amount bigint DEFAULT 500000,term_days int DEFAULT 30,product_policy_version int DEFAULT 1,repayment_interval_days int DEFAULT 30,grace_days int DEFAULT 3,allocation_order text[] DEFAULT ARRAY['principal'],submitted_at timestamptz DEFAULT '2026-09-01');
 CREATE TEMP TABLE credit_application_environments(application_id text PRIMARY KEY,environment text,caller_application_id text NOT NULL DEFAULT '');
 CREATE TEMP TABLE credit_application_drafts(application_id text PRIMARY KEY,caller_application_id text);
 INSERT INTO credit_applications(id,tenant_id) VALUES('a','tenant-a'),('b','tenant-a'),('c','tenant-b'),('d','tenant-a'),('e','tenant-a'),('f','tenant-a');
 INSERT INTO credit_application_environments(application_id,environment) VALUES('a','sandbox'),('b','sandbox'),('c','sandbox'),('d','production'),('e','sandbox');
 INSERT INTO credit_application_drafts VALUES('a','app-a'),('e','app-b');`)
	if err != nil {
		t.Fatal(err)
	}
	s := Postgres{Pool: pool}
	scope := creditrisk.ApplicationListScope{TenantID: "tenant-a", Environment: "sandbox", CallerApplicationID: "app-a"}
	_, err = pool.Exec(ctx, `CREATE TEMP TABLE credit_decision_audit(id bigint,application_id text,action text,reason text,actor text,created_at timestamptz DEFAULT now());
	INSERT INTO credit_decision_audit(id,application_id,action,reason,actor) VALUES(1,'a','submitted','private','staff'),(2,'a','internal_note','secret','staff'),(3,'a','offer','private','staff'),(4,'c','submitted','private','staff'),(5,'e','submitted','private','staff');`)
	if err != nil {
		t.Fatal(err)
	}
	events, err := s.TenantTimeline(ctx, scope, "a", "", 1)
	if err != nil || len(events) != 1 || events[0].ID != "1" {
		t.Fatal(events, err)
	}
	events, err = s.TenantTimeline(ctx, scope, "a", "1", 10)
	if err != nil || len(events) != 1 || events[0].ID != "3" {
		t.Fatal(events, err)
	}
	for _, cursor := range []string{"2", "4", "5", "999", "invalid", "-1"} {
		if _, err = s.TenantTimeline(ctx, scope, "a", cursor, 10); !errors.Is(err, creditrisk.ErrInvalidCursor) {
			t.Fatal("timeline cursor", cursor, err)
		}
	}
	for _, id := range []string{"c", "d", "e", "f"} {
		events, err = s.TenantTimeline(ctx, scope, id, "", 10)
		if err != nil || len(events) != 0 {
			t.Fatal("timeline isolation", id, events, err)
		}
	}
	rows, err := s.ListApplications(ctx, scope, creditrisk.ApplicationListFilter{Limit: 1})
	if err != nil || len(rows) != 1 || rows[0].ID != "a" {
		t.Fatal(rows, err)
	}
	rows, err = s.ListApplications(ctx, scope, creditrisk.ApplicationListFilter{Limit: 10, Cursor: "a"})
	if err != nil || len(rows) != 1 || rows[0].ID != "b" {
		t.Fatal(rows, err)
	}
	for _, cursor := range []string{"c", "d", "e", "f", "missing"} {
		_, err = s.ListApplications(ctx, scope, creditrisk.ApplicationListFilter{Limit: 10, Cursor: cursor})
		if !errors.Is(err, creditrisk.ErrInvalidCursor) {
			t.Fatalf("cursor %s: %v", cursor, err)
		}
	}
	rows, err = s.ListApplications(ctx, scope, creditrisk.ApplicationListFilter{Limit: 10, Status: "accepted"})
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	rows, err = s.ListApplications(ctx, scope, creditrisk.ApplicationListFilter{Limit: 10, RelationshipID: "foreign"})
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	scope.CallerApplicationID = ""
	rows, err = s.ListApplications(ctx, scope, creditrisk.ApplicationListFilter{Limit: 10})
	if err != nil || len(rows) != 3 {
		t.Fatal(rows, err)
	}
}
