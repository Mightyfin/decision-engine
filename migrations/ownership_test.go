package migrations

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPopulatedOwnershipMigration(t *testing.T) {
	dsn := os.Getenv("DECISION_ENGINE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("ownership_upgrade_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE") }()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	// Reproduce the populated schema immediately before version 16, including
	// the immutable environment trigger rather than only testing an empty DB.
	_, err = p.Exec(ctx, `CREATE TABLE decision_engine_schema_migrations(version text PRIMARY KEY,applied_at timestamptz DEFAULT now());
	 CREATE TABLE credit_applications(id text PRIMARY KEY,tenant_id text);
	 CREATE TABLE credit_application_environments(application_id text PRIMARY KEY,environment text);
	 CREATE TABLE credit_submission_replays(response jsonb,caller_application_id text,tenant_id text,environment text);
	 CREATE FUNCTION guard_scope() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'immutable'; END $$;
	 CREATE TRIGGER credit_environment_immutable BEFORE UPDATE OR DELETE ON credit_application_environments FOR EACH ROW EXECUTE FUNCTION guard_scope();
	 INSERT INTO credit_applications VALUES('case_a','ten_a'),('unknown','ten_a');
	 INSERT INTO credit_application_environments VALUES('case_a','sandbox'),('unknown','sandbox');
	 INSERT INTO credit_submission_replays VALUES('{"id":"case_a"}','app_owner','ten_a','sandbox');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = apply(ctx, p, "00016_submission_ownership", submissionOwnership); err != nil {
		t.Fatal(err)
	}
	var owner string
	if err = p.QueryRow(ctx, `SELECT caller_application_id FROM credit_application_environments WHERE application_id='case_a'`).Scan(&owner); err != nil || owner != "app_owner" {
		t.Fatal("backfill", owner, err)
	}
	if err = p.QueryRow(ctx, `SELECT caller_application_id FROM credit_application_environments WHERE application_id='unknown'`).Scan(&owner); err != nil || owner != "" {
		t.Fatal("invented owner", owner, err)
	}
	if _, err = p.Exec(ctx, `UPDATE credit_application_environments SET environment='production' WHERE application_id='case_a'`); err == nil {
		t.Fatal("scope trigger left disabled")
	}
	if err = apply(ctx, p, "00016_submission_ownership", submissionOwnership); err != nil {
		t.Fatal("rerun", err)
	}
}
