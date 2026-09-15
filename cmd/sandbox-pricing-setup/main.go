// sandbox-pricing-setup is an attributed operator setup, not a staff approval.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Mightyfin/decision-engine/pricing"
	"github.com/Mightyfin/decision-engine/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if os.Getenv("DECISION_ENGINE_ENVIRONMENT") != "sandbox" || os.Getenv("CONFIRM_SANDBOX_SETUP") != "yes" {
		return fmt.Errorf("explicit sandbox setup required")
	}
	var in struct {
		TenantID, Actor string
		Policy          pricing.Policy
		ProductIDs      []string
	}
	dec := json.NewDecoder(os.Stdin)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return err
	}
	if !strings.HasPrefix(in.TenantID, "ten_") || !strings.HasPrefix(in.Actor, "operator:") || !strings.HasPrefix(in.Policy.ProductPolicyID, "uat_") || in.Policy.Currency != "ZMW" || len(in.ProductIDs) == 0 {
		return fmt.Errorf("synthetic policy and attributed operator required")
	}
	in.Policy.CreatedBy = in.Actor
	in.Policy.Active = true
	in.Policy.ProductPolicyVersion = 1
	if _, err := pricing.QuoteFor(in.Policy, pricing.ScheduleTerms{RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}}, 100000, 30, time.Now()); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("DECISION_ENGINE_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	s := storage.Postgres{Pool: pool}
	if err = s.CreatePricingPolicy(ctx, pricing.DefaultOwner, in.Policy); err != nil {
		return err
	}
	for _, id := range in.ProductIDs {
		if err = s.BindDefaultPricing(ctx, in.TenantID, id, in.Actor, pricing.DefaultBinding{DefaultPolicyKey: in.Policy.ProductPolicyID, Currency: in.Policy.Currency}); err != nil {
			return err
		}
		p, err := s.PricingPolicy(ctx, in.TenantID, id)
		if err != nil {
			return err
		}
		if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"product_id": id, "source": p.Source, "version": p.Version, "rate_bps": p.InterestRateBPS}); err != nil {
			return err
		}
	}
	return nil
}
