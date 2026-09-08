package storage

import (
	"context"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
)

func (s Postgres) ReviewHistory(ctx context.Context, tenant, id string) ([]creditrisk.Audit, error) {
	rows, err := s.Pool.Query(ctx, `SELECT a.application_id,a.actor,a.action,a.reason,a.created_at FROM credit_decision_audit a JOIN credit_applications c ON c.id=a.application_id WHERE c.tenant_id=$1 AND c.id=$2 ORDER BY a.created_at,a.id`, tenant, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []creditrisk.Audit{}
	for rows.Next() {
		var a creditrisk.Audit
		if err := rows.Scan(&a.ApplicationID, &a.Actor, &a.Action, &a.Reason, &a.At); err != nil {
			return nil, err
		}
		events = append(events, a)
	}
	return events, rows.Err()
}
