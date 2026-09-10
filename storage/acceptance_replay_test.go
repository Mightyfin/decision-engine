package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOfferAcceptanceDurableReplay(t *testing.T) {
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
	schema := fmt.Sprintf("acceptance_test_%d", time.Now().UnixNano())
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
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = migrations.Up(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Up(ctx, pool); err != nil {
		t.Fatal("migration rerun", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO credit_applications(id,tenant_id,product_policy_id,relationship_id,amount,currency,term_days,purpose,product_policy_version,status) VALUES('a','t','p','r',10000,'ZMW',30,'test',1,'offered');
 INSERT INTO credit_application_environments(application_id,environment) VALUES('a','sandbox');
 INSERT INTO credit_offers(application_id,quote_id,product_policy_version,pricing_policy_version,principal,interest,fees,total,term_days,expires_at) VALUES('a','q',1,1,10000,1000,0,11000,30,now()+interval '1 day');`)
	if err != nil {
		t.Fatal(err)
	}
	s := Postgres{Pool: pool}
	a, err := s.Application(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	a.Environment = "sandbox"
	offer, err := s.Offer(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	audit := creditrisk.Audit{ApplicationID: "a", Actor: "tenant-user", Action: "accepted", At: time.Now()}
	in := creditrisk.AcceptanceRequest{TenantID: "t", Environment: "sandbox", CallerApplicationID: "client", Key: "stable-acceptance-key", QuoteID: "q", ConsentReference: "consent-123"}
	wrong := in
	wrong.QuoteID = "stale"
	if _, _, err = s.AcceptOffer(ctx, a, offer, audit, wrong); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("stale quote", err)
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE credit_outbox ADD CONSTRAINT test_delivery_failure CHECK(event_type <> 'credit.offer.accepted')`); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AcceptOffer(ctx, a, offer, audit, in); err == nil {
		t.Fatal("expected outbox failure")
	}
	var current string
	if err = pool.QueryRow(ctx, `SELECT status FROM credit_applications WHERE id='a'`).Scan(&current); err != nil || current != "offered" {
		t.Fatal("partial acceptance", current, err)
	}
	for _, table := range []string{"credit_decision_audit", "credit_offer_acceptance_replays"} {
		var count int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal("partial transaction", table, count, err)
		}
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE credit_outbox DROP CONSTRAINT test_delivery_failure`); err != nil {
		t.Fatal(err)
	}
	// Same key succeeds after the rejected transaction, and concurrent retries
	// converge on one audit, one outbox event and the same response snapshot.
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _, e := s.AcceptOffer(ctx, a, offer, audit, in)
			if e == nil && got.Status != "accepted" {
				e = fmt.Errorf("unexpected state %s", got.Status)
			}
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	for _, table := range []string{"credit_decision_audit", "credit_outbox", "credit_offer_acceptance_replays"} {
		var count int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 1 {
			t.Fatal(table, count, err)
		}
	}
	// Expiry after acceptance must not destroy the original successful replay.
	if _, err = pool.Exec(ctx, `UPDATE credit_offers SET expires_at=now()-interval '1 day'`); err != nil {
		t.Fatal(err)
	}
	got, replayed, err := s.AcceptOffer(ctx, a, offer, audit, in)
	if err != nil || !replayed || got.Status != "accepted" {
		t.Fatal(got, replayed, err)
	}
	changed := in
	changed.ConsentReference = "different"
	if _, _, err = s.AcceptOffer(ctx, a, offer, audit, changed); !errors.Is(err, creditrisk.ErrAcceptanceConflict) {
		t.Fatal("changed consent", err)
	}
	changed = in
	changed.Key = "a-different-operation"
	if _, _, err = s.AcceptOffer(ctx, a, offer, audit, changed); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("second acceptance", err)
	}
	changed = in
	changed.TenantID = "foreign"
	if _, _, err = s.AcceptOffer(ctx, a, offer, audit, changed); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("foreign tenant", err)
	}
	changed = in
	changed.Environment = "production"
	if _, _, err = s.AcceptOffer(ctx, a, offer, audit, changed); !errors.Is(err, creditrisk.ErrInvalidState) {
		t.Fatal("foreign environment", err)
	}
	t.Run("submission retries", func(t *testing.T) { testSubmissionRetries(t, pool) })
	t.Run("cancellation guards", func(t *testing.T) { testCancellationGuards(t, pool) })
	t.Run("commercial review", func(t *testing.T) { testCommercialReview(t, pool) })
	t.Run("tenant lifecycle events", func(t *testing.T) { testTenantEvents(t, pool) })
	t.Run("immutable usage snapshots", func(t *testing.T) { testUsageSnapshots(t, pool) })
}
