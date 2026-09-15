package storage

import (
	"context"
	"errors"
	"fmt"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/migrations"
	"github.com/Mightyfin/decision-engine/pricing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

func TestDefaultPricingResolution(t *testing.T) {
	dsn := os.Getenv("DECISION_DEFAULT_PRICING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable database required")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("pricing_uat_%d", time.Now().UnixNano())
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
	s := Postgres{Pool: pool}
	p := pricing.Policy{ProductPolicyID: "shared-credit", Version: 1, InterestMethod: "flat", RatePeriod: "monthly", InterestRateBPS: 200, PenaltyBasis: "overdue_principal", Currency: "ZMW", CreatedBy: "staff-test", Active: true}
	if err = s.CreatePricingPolicy(ctx, pricing.DefaultOwner, p); err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []string{"green-test", "other-test"} {
		if err = s.BindDefaultPricing(ctx, tenant, "product-a", "staff-test", pricing.DefaultBinding{DefaultPolicyKey: p.ProductPolicyID, Currency: "ZMW"}); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := s.PricingPolicy(ctx, "green-test", "product-a")
	if err != nil || resolved.InterestRateBPS != 200 || resolved.Source != "mightyfin_default" || resolved.ProductPolicyID != "product-a" {
		t.Fatal(resolved, err)
	}
	resolved.ProductPolicyVersion = 1
	terms := pricing.ScheduleTerms{RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}}
	old, err := pricing.QuoteFor(resolved, terms, 100000, 30, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p.Version = 2
	p.InterestRateBPS = 300
	if err = s.CreatePricingPolicy(ctx, pricing.DefaultOwner, p); err != nil {
		t.Fatal(err)
	}
	resolved, err = s.PricingPolicy(ctx, "green-test", "product-a")
	if err != nil || resolved.Version != 2 || resolved.InterestRateBPS != 300 {
		t.Fatal(resolved, err)
	}
	if old.Interest != 2000 || old.ChargeLines[0].PricingVersion != 1 || old.ChargeLines[0].PricingPolicyKey != "shared-credit" {
		t.Fatal("old quote changed", old)
	}
	p.ProductPolicyID = "product-a"
	p.Version = 1
	p.InterestRateBPS = 100
	if err = s.CreatePricingPolicy(ctx, "green-test", p); err != nil {
		t.Fatal(err)
	}
	resolved, err = s.PricingPolicy(ctx, "green-test", "product-a")
	if err != nil || resolved.Source != "tenant_override" || resolved.InterestRateBPS != 100 {
		t.Fatal(resolved, err)
	}
	resolved, err = s.PricingPolicy(ctx, "other-test", "product-a")
	if err != nil || resolved.Source != "mightyfin_default" || resolved.InterestRateBPS != 300 {
		t.Fatal("tenant leak", resolved, err)
	}
	// Same product/version in another tenant must not collide with Green's row.
	if err = s.CreatePricingPolicy(ctx, "third-test", p); err != nil {
		t.Fatal(err)
	}
	if err = s.CreatePricingPolicy(ctx, "green-test", p); err == nil {
		t.Fatal("repeated version accepted")
	}
	p.Version = 2
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- s.CreatePricingPolicy(ctx, "green-test", p) }()
	}
	successes := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("concurrent publication successes", successes)
	}
	for _, tenant := range []string{"unassigned", pricing.DefaultOwner, ""} {
		if _, err = s.PricingPolicy(ctx, tenant, "product-a"); !errors.Is(err, creditrisk.ErrNotFound) {
			t.Fatal("unexpected fallback", tenant, err)
		}
	}
	if err = s.BindDefaultPricing(ctx, "green-test", "wrong-currency", "staff-test", pricing.DefaultBinding{DefaultPolicyKey: "shared-credit", Currency: "USD"}); err == nil {
		t.Fatal("currency mismatch accepted")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM credit_pricing_configuration_audit`); err == nil {
		t.Fatal("audit mutation allowed")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM credit_pricing_configuration_audit`).Scan(&count); err != nil || count != 7 {
		t.Fatal("audit count", count, err)
	}
}
