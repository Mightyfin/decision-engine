// Package storage contains persistence adapters; it does not contain lending decisions.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/pricing"
	"github.com/Mightyfin/decision-engine/product"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ Pool *pgxpool.Pool }

func (s Postgres) CreateProductPolicy(ctx context.Context, p product.Policy) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO product_policies(id,tenant_id,code,currency,version,minimum_amount,maximum_amount,minimum_term_days,maximum_term_days,active) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,true)`, p.ID, p.TenantID, p.Code, p.Currency, p.Version, p.MinimumAmount, p.MaximumAmount, p.MinimumTermDays, p.MaximumTermDays)
	return err
}

func (s Postgres) CreatePricingPolicy(ctx context.Context, tenantID string, p pricing.Policy) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE pricing_policies SET active=false WHERE tenant_id=$1 AND product_policy_id=$2 AND active=true`, tenantID, p.ProductPolicyID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO pricing_policies(id,tenant_id,product_policy_id,version,annual_rate_bps,origination_fee_bps,active) VALUES($1,$2,$3,$4,$5,$6,true)`, fmt.Sprintf("prc_%s_%d", p.ProductPolicyID, p.Version), tenantID, p.ProductPolicyID, p.Version, p.AnnualRateBPS, p.OriginationFeeBPS)
		return err
	})
}

func (s Postgres) CreateApplication(ctx context.Context, a creditrisk.Application, audit creditrisk.Audit) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if err := saveApplication(ctx, tx, a); err != nil {
			return err
		}
		return appendAudit(ctx, tx, audit)
	})
}

func (s Postgres) RecordDecision(ctx context.Context, a creditrisk.Application, offer *creditrisk.Offer, audit creditrisk.Audit) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if offer != nil {
			if err := saveOffer(ctx, tx, *offer); err != nil {
				return err
			}
		}
		if err := saveApplication(ctx, tx, a); err != nil {
			return err
		}
		return appendAudit(ctx, tx, audit)
	})
}

func (s Postgres) RecordAcceptance(ctx context.Context, a creditrisk.Application, audit creditrisk.Audit) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if err := saveApplication(ctx, tx, a); err != nil {
			return err
		}
		return appendAudit(ctx, tx, audit)
	})
}
func (s Postgres) RecordAcceptanceWithEvent(ctx context.Context, a creditrisk.Application, audit creditrisk.Audit, offer creditrisk.Offer) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if err := saveApplication(ctx, tx, a); err != nil {
			return err
		}
		if err := appendAudit(ctx, tx, audit); err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{"application_id": a.ID, "tenant_id": a.TenantID, "offer_quote_id": offer.QuoteID, "currency": a.Currency, "total_minor": offer.Total})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO credit_outbox(event_type,aggregate_id,tenant_id,payload,occurred_at) VALUES('credit.offer.accepted',$1,$2,$3,$4)`, a.ID, a.TenantID, payload, audit.At)
		return err
	})
}

func (s Postgres) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s Postgres) Policy(ctx context.Context, tenantID, id string) (product.Policy, error) {
	var p product.Policy
	err := s.Pool.QueryRow(ctx, `SELECT id,tenant_id,code,currency,version,minimum_amount,maximum_amount,minimum_term_days,maximum_term_days,active FROM product_policies WHERE id=$1 AND tenant_id=$2`, id, tenantID).Scan(&p.ID, &p.TenantID, &p.Code, &p.Currency, &p.Version, &p.MinimumAmount, &p.MaximumAmount, &p.MinimumTermDays, &p.MaximumTermDays, &p.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, product.ErrNotFound
	}
	return p, err
}

// PricingPolicy returns the exact active policy selected by an analyst. Rates never arrive from
// an EFaaS tenant request or a browser form.
func (s Postgres) PricingPolicy(ctx context.Context, tenantID, productPolicyID string) (pricing.Policy, error) {
	var p pricing.Policy
	err := s.Pool.QueryRow(ctx, `SELECT product_policy_id,version,annual_rate_bps,origination_fee_bps,active FROM pricing_policies WHERE tenant_id=$1 AND product_policy_id=$2 AND active=true ORDER BY version DESC LIMIT 1`, tenantID, productPolicyID).Scan(&p.ProductPolicyID, &p.Version, &p.AnnualRateBPS, &p.OriginationFeeBPS, &p.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, creditrisk.ErrNotFound
	}
	if err != nil {
		return p, err
	}
	productPolicy, err := s.Policy(ctx, tenantID, productPolicyID)
	if err != nil {
		return p, err
	}
	p.ProductPolicyVersion, p.Currency = productPolicy.Version, productPolicy.Currency
	return p, nil
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
	return saveApplication(ctx, s.Pool, a)
}
func (s Postgres) SaveOffer(ctx context.Context, o creditrisk.Offer) error {
	return saveOffer(ctx, s.Pool, o)
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
	return appendAudit(ctx, s.Pool, a)
}

type sqlExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func saveApplication(ctx context.Context, db sqlExecutor, a creditrisk.Application) error {
	_, err := db.Exec(ctx, `INSERT INTO credit_applications(id,tenant_id,product_policy_id,relationship_id,amount,currency,term_days,purpose,product_policy_version,status,submitted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(id) DO UPDATE SET status=EXCLUDED.status`, a.ID, a.TenantID, a.ProductPolicyID, a.RelationshipID, a.Amount, a.Currency, a.TermDays, a.Purpose, a.ProductPolicyVersion, a.Status, a.SubmittedAt)
	return err
}
func saveOffer(ctx context.Context, db sqlExecutor, o creditrisk.Offer) error {
	_, err := db.Exec(ctx, `INSERT INTO credit_offers(application_id,quote_id,product_policy_version,pricing_policy_version,principal,interest,fees,total,term_days,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, o.ApplicationID, o.QuoteID, o.ProductPolicyVersion, o.PricingPolicyVersion, o.Principal, o.Interest, o.Fees, o.Total, o.TermDays, o.ExpiresAt)
	return err
}
func appendAudit(ctx context.Context, db sqlExecutor, a creditrisk.Audit) error {
	if a.At.IsZero() {
		a.At = time.Now().UTC()
	}
	_, err := db.Exec(ctx, `INSERT INTO credit_decision_audit(application_id,actor,action,reason,created_at) VALUES($1,$2,$3,$4,$5)`, a.ApplicationID, a.Actor, a.Action, a.Reason, a.At)
	return err
}
