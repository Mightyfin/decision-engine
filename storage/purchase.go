package storage

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (s Postgres) PurchaseRestriction(ctx context.Context, id, tenant, environment string) (creditrisk.PurchaseRestriction, error) {
	var p creditrisk.PurchaseRestriction
	err := s.Pool.QueryRow(ctx, `SELECT p.application_id,p.order_reference,p.supplier_party_id,p.destination_wallet_id,p.document_id,p.sha256,p.currency,p.maximum_amount_minor,p.recorded_by,p.recorded_at FROM credit_purchase_restrictions p JOIN credit_applications a ON a.id=p.application_id JOIN credit_application_environments e ON e.application_id=a.id WHERE a.id=$1 AND a.tenant_id=$2 AND e.environment=$3`, id, tenant, environment).Scan(&p.ApplicationID, &p.OrderReference, &p.SupplierPartyID, &p.DestinationWalletID, &p.DocumentID, &p.SHA256, &p.Currency, &p.MaximumAmountMinor, &p.RecordedBy, &p.RecordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, creditrisk.ErrNotFound
	}
	return p, err
}

func (s Postgres) RecordPurchaseRestriction(ctx context.Context, p creditrisk.PurchaseRestriction, tenant, environment, actor, reason, version string) error {
	if p.Validate() != nil || tenant == "" || environment == "" || strings.TrimSpace(actor) == "" || len(strings.TrimSpace(reason)) < 10 || len(reason) > 4000 {
		return creditrisk.ErrInvalidState
	}
	return s.withTx(ctx, func(tx pgx.Tx) error {
		var status, currency, party string
		var amount int64
		err := tx.QueryRow(ctx, `SELECT a.status,a.currency,a.amount,coalesce(a.party_id,'') FROM credit_applications a JOIN credit_application_environments e ON e.application_id=a.id WHERE a.id=$1 AND a.tenant_id=$2 AND e.environment=$3 FOR UPDATE OF a`, p.ApplicationID, tenant, environment).Scan(&status, &currency, &amount, &party)
		if errors.Is(err, pgx.ErrNoRows) {
			return creditrisk.ErrNotFound
		}
		if err != nil {
			return err
		}
		if status != "pending_review" || currency != p.Currency || amount != p.MaximumAmountMinor || party == "" || party == p.SupplierPartyID {
			return creditrisk.ErrInvalidState
		}
		if err = checkReviewVersion(ctx, tx, p.ApplicationID, version); err != nil {
			return err
		}
		var valid bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credit_application_evidence WHERE application_id=$1 AND tenant_id=$2 AND environment=$3 AND party_id=$4 AND document_id=$5 AND sha256=$6 AND document_type IN ('invoice','purchase_order'))`, p.ApplicationID, tenant, environment, party, p.DocumentID, p.SHA256).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid {
			return creditrisk.ErrEvidenceScope
		}
		tag, err := tx.Exec(ctx, `INSERT INTO credit_purchase_restrictions(application_id,order_reference,supplier_party_id,destination_wallet_id,document_id,sha256,currency,maximum_amount_minor,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(application_id) DO NOTHING`, p.ApplicationID, p.OrderReference, p.SupplierPartyID, p.DestinationWalletID, p.DocumentID, p.SHA256, p.Currency, p.MaximumAmountMinor, actor)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return creditrisk.ErrInvalidState
		}
		return appendAudit(ctx, tx, creditrisk.Audit{ApplicationID: p.ApplicationID, Actor: actor, Action: "purchase_restriction_recorded", Reason: strings.TrimSpace(reason)})
	})
}
