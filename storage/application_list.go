package storage

import (
	"context"
	"fmt"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
)

func (s Postgres) ListApplications(ctx context.Context, scope creditrisk.ApplicationListScope, filter creditrisk.ApplicationListFilter) ([]creditrisk.Application, error) {
	if scope.TenantID == "" || scope.Environment == "" || filter.Limit < 1 || filter.Limit > 101 {
		return nil, fmt.Errorf("invalid list scope or limit")
	}
	// Inner environment join fails closed for legacy records without environment provenance.
	const visible = ` FROM credit_applications a JOIN credit_application_environments e ON e.application_id=a.id
	 LEFT JOIN credit_application_drafts d ON d.application_id=a.id
	 WHERE a.tenant_id=$1 AND e.environment=$2 AND ($3='' OR (COALESCE(d.caller_application_id,'') IN ('',$3) AND e.caller_application_id IN ('',$3)))`
	args := []any{scope.TenantID, scope.Environment, scope.CallerApplicationID}
	if filter.Cursor != "" {
		var exists bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1`+visible+` AND a.id=$4)`, append(args, filter.Cursor)...).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, creditrisk.ErrInvalidCursor
		}
	}
	rows, err := s.Pool.Query(ctx, `SELECT a.id,a.tenant_id,e.environment,a.product_policy_id,a.relationship_id,a.currency,a.purpose,a.status,a.amount,a.term_days,a.submitted_at,COALESCE(a.party_id,''),COALESCE(a.applicant_role,''),COALESCE(a.wallet_id,''),COALESCE(a.origin,''),a.product_policy_version,a.repayment_interval_days,a.grace_days,a.allocation_order,a.usage_terms`+visible+`
	 AND ($4='' OR a.id>$4) AND ($5='' OR a.status=$5) AND ($6='' OR a.relationship_id=$6)
	 ORDER BY a.id LIMIT $7`, append(args, filter.Cursor, filter.Status, filter.RelationshipID, filter.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []creditrisk.Application{}
	for rows.Next() {
		var a creditrisk.Application
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Environment, &a.ProductPolicyID, &a.RelationshipID, &a.Currency, &a.Purpose, &a.Status, &a.Amount, &a.TermDays, &a.SubmittedAt, &a.PartyID, &a.ApplicantRole, &a.WalletID, &a.Origin, &a.ProductPolicyVersion, &a.RepaymentIntervalDays, &a.GraceDays, &a.AllocationOrder, &a.UsageTerms); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
