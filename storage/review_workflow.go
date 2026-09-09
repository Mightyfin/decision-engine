package storage

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (s Postgres) ReviewRevision(ctx context.Context, tenant, id string) (string, error) {
	var version string
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(max(h.id),0)::text FROM credit_decision_audit h JOIN credit_applications a ON a.id=h.application_id WHERE a.tenant_id=$1 AND a.id=$2`, tenant, id).Scan(&version)
	return version, err
}

func (s Postgres) InformationMessage(ctx context.Context, tenant, id string) (string, error) {
	var message string
	err := s.Pool.QueryRow(ctx, `SELECT h.reason FROM credit_decision_audit h JOIN credit_applications a ON a.id=h.application_id WHERE a.tenant_id=$1 AND a.id=$2 AND h.action='information_requested' ORDER BY h.id DESC LIMIT 1`, tenant, id).Scan(&message)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return message, err
}

// All review-changing mutations serialize on the application row. Compare the
// version after taking the lock, not before waiting for another writer.
func checkReviewVersion(ctx context.Context, tx pgx.Tx, id, version string) error {
	if version == "" {
		return creditrisk.ErrInvalidState
	}
	var current string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(id),0)::text FROM credit_decision_audit WHERE application_id=$1`, id).Scan(&current); err != nil {
		return err
	}
	if current != version {
		return creditrisk.ErrInvalidState
	}
	return nil
}

func (s Postgres) ChangeInformationState(ctx context.Context, id, tenant, environment, actor, reason, version string, resubmit bool) error {
	if strings.TrimSpace(actor) == "" || len(strings.TrimSpace(reason)) < 10 || len(reason) > 4000 || version == "" || environment == "" {
		return creditrisk.ErrInvalidState
	}
	return s.withTx(ctx, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT a.status FROM credit_applications a JOIN credit_application_environments e ON e.application_id=a.id WHERE a.id=$1 AND a.tenant_id=$2 AND e.environment=$3 FOR UPDATE OF a`, id, tenant, environment).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return creditrisk.ErrNotFound
		}
		if err != nil {
			return err
		}
		from, to, action := "pending_review", "awaiting_information", "information_requested"
		if resubmit {
			from, to, action = "awaiting_information", "pending_review", "information_resubmitted"
		}
		if status != from {
			return creditrisk.ErrInvalidState
		}
		if err = checkReviewVersion(ctx, tx, id, version); err != nil {
			return err
		}
		if resubmit {
			var revision int
			var required bool
			err = tx.QueryRow(ctx, `SELECT revision,COALESCE((requirements->>'commercial_review_required')::boolean,false) FROM credit_application_drafts WHERE application_id=$1`, id).Scan(&revision, &required)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if required {
				if err = commercialApproved(ctx, tx, id, revision); err != nil {
					return err
				}
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE credit_applications SET status=$2 WHERE id=$1`, id, to); err != nil {
			return err
		}
		return appendAudit(ctx, tx, creditrisk.Audit{ApplicationID: id, Actor: actor, Action: action, Reason: strings.TrimSpace(reason)})
	})
}
