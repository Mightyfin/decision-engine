package storage

import (
	"context"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"strconv"
)

func (s Postgres) TenantTimeline(ctx context.Context, sc creditrisk.ApplicationListScope, id, cursor string, limit int) ([]creditrisk.TimelineEvent, error) {
	if sc.TenantID == "" || sc.Environment == "" || limit < 1 || limit > 101 {
		return nil, creditrisk.ErrNotFound
	}
	const visible = ` FROM credit_decision_audit h JOIN credit_applications a ON a.id=h.application_id
	 JOIN credit_application_environments e ON e.application_id=a.id
	 LEFT JOIN credit_application_drafts d ON d.application_id=a.id
	 WHERE a.tenant_id=$1 AND e.environment=$2 AND a.id=$3
	 AND ($4='' OR (COALESCE(d.caller_application_id,'') IN ('',$4) AND e.caller_application_id IN ('',$4)))
	 AND h.action IN ('draft_created','draft_updated','submitted','information_requested','information_resubmitted','offer','decline','accepted','cancelled','commercial_approved','commercial_rejected')`
	var after int64
	args := []any{sc.TenantID, sc.Environment, id, sc.CallerApplicationID}
	if cursor != "" {
		var err error
		after, err = strconv.ParseInt(cursor, 10, 64)
		if err != nil || after < 1 || strconv.FormatInt(after, 10) != cursor {
			return nil, creditrisk.ErrInvalidCursor
		}
		var found bool
		if err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1`+visible+` AND h.id=$5)`, append(args, after)...).Scan(&found); err != nil {
			return nil, err
		}
		if !found {
			return nil, creditrisk.ErrInvalidCursor
		}
	}
	rows, err := s.Pool.Query(ctx, `SELECT h.id::text,h.action,h.created_at`+visible+` AND h.id>$5 ORDER BY h.id LIMIT $6`, append(args, after, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []creditrisk.TimelineEvent{}
	for rows.Next() {
		var event creditrisk.TimelineEvent
		if err := rows.Scan(&event.ID, &event.Action, &event.At); err != nil {
			return nil, err
		}
		items = append(items, event)
	}
	return items, rows.Err()
}
