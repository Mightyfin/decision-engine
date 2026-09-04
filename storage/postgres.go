// Package storage contains persistence adapters; it does not contain lending decisions.
package storage

import (
	"context"
	"errors"
	"time"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/product"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ Pool *pgxpool.Pool }

func (s Postgres) Policy(ctx context.Context, tenantID, id string) (product.Policy, error) {
	var p product.Policy
	err := s.Pool.QueryRow(ctx, `SELECT id,tenant_id,code,currency,version,minimum_amount,maximum_amount,minimum_term_days,maximum_term_days,active FROM product_policies WHERE id=$1 AND tenant_id=$2`, id, tenantID).Scan(&p.ID, &p.TenantID, &p.Code, &p.Currency, &p.Version, &p.MinimumAmount, &p.MaximumAmount, &p.MinimumTermDays, &p.MaximumTermDays, &p.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, product.ErrNotFound
	}
	return p, err
}
func (s Postgres) Application(ctx context.Context, id string) (creditrisk.Application, error) {
	var a creditrisk.Application
	err := s.Pool.QueryRow(ctx, `SELECT id,tenant_id,product_policy_id,relationship_id,currency,purpose,status,amount,term_days,product_policy_version,submitted_at FROM credit_applications WHERE id=$1`, id).Scan(&a.ID, &a.TenantID, &a.ProductPolicyID, &a.RelationshipID, &a.Currency, &a.Purpose, &a.Status, &a.Amount, &a.TermDays, &a.ProductPolicyVersion, &a.SubmittedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, creditrisk.ErrNotFound
	}
	return a, err
}
func (s Postgres) ReviewQueue(ctx context.Context, tenantID string, limit int) ([]creditrisk.Application, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `SELECT id,tenant_id,product_policy_id,relationship_id,currency,purpose,status,amount,term_days,product_policy_version,submitted_at FROM credit_applications WHERE tenant_id=$1 AND status='pending_review' ORDER BY submitted_at,id LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []creditrisk.Application{}
	for rows.Next() {
		var a creditrisk.Application
		if err = rows.Scan(&a.ID, &a.TenantID, &a.ProductPolicyID, &a.RelationshipID, &a.Currency, &a.Purpose, &a.Status, &a.Amount, &a.TermDays, &a.ProductPolicyVersion, &a.SubmittedAt); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
func (s Postgres) SaveApplication(ctx context.Context, a creditrisk.Application) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO credit_applications(id,tenant_id,product_policy_id,relationship_id,amount,currency,term_days,purpose,product_policy_version,status,submitted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(id) DO UPDATE SET status=EXCLUDED.status`, a.ID, a.TenantID, a.ProductPolicyID, a.RelationshipID, a.Amount, a.Currency, a.TermDays, a.Purpose, a.ProductPolicyVersion, a.Status, a.SubmittedAt)
	return err
}
func (s Postgres) SaveOffer(ctx context.Context, o creditrisk.Offer) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO credit_offers(application_id,quote_id,product_policy_version,pricing_policy_version,principal,interest,fees,total,term_days,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, o.ApplicationID, o.QuoteID, o.ProductPolicyVersion, o.PricingPolicyVersion, o.Principal, o.Interest, o.Fees, o.Total, o.TermDays, o.ExpiresAt)
	return err
}
func (s Postgres) Offer(ctx context.Context, id string) (creditrisk.Offer, error) {
	var o creditrisk.Offer
	err := s.Pool.QueryRow(ctx, `SELECT application_id,quote_id,product_policy_version,pricing_policy_version,principal,interest,fees,total,term_days,expires_at FROM credit_offers WHERE application_id=$1`, id).Scan(&o.ApplicationID, &o.QuoteID, &o.ProductPolicyVersion, &o.PricingPolicyVersion, &o.Principal, &o.Interest, &o.Fees, &o.Total, &o.TermDays, &o.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, creditrisk.ErrNotFound
	}
	return o, err
}
func (s Postgres) Exposure(context.Context, string, string) (creditrisk.Exposure, error) {
	return creditrisk.Exposure{}, nil
}
func (s Postgres) AppendAudit(ctx context.Context, a creditrisk.Audit) error {
	if a.At.IsZero() {
		a.At = time.Now().UTC()
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO credit_decision_audit(application_id,actor,action,reason,created_at) VALUES($1,$2,$3,$4,$5)`, a.ApplicationID, a.Actor, a.Action, a.Reason, a.At)
	return err
}
