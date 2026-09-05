// Package eventbus delivers committed domain events. It never makes credit decisions.
package eventbus

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const claimLease = 5 * time.Minute

// Event is a committed outbox record waiting for durable publication.
type Event struct {
	ID          int64
	Type        string
	AggregateID string
	TenantID    string
	Payload     json.RawMessage
	OccurredAt  time.Time
}

type Store struct{ Pool *pgxpool.Pool }

// Claim obtains one publishable event. SKIP LOCKED permits multiple workers without
// duplicate concurrent delivery; a lease makes a crashed worker's claim recoverable.
func (s Store) Claim(ctx context.Context) (*Event, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var event Event
	err = tx.QueryRow(ctx, `
WITH candidate AS (
  SELECT id
  FROM credit_outbox
  WHERE published_at IS NULL
    AND available_at <= now()
    AND (publishing_at IS NULL OR publishing_at < now() - ($1 * interval '1 millisecond'))
  ORDER BY id
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE credit_outbox outbox
SET publishing_at = now(), publish_attempts = publish_attempts + 1
FROM candidate
WHERE outbox.id = candidate.id
RETURNING outbox.id, outbox.event_type, outbox.aggregate_id, outbox.tenant_id,
          outbox.payload, outbox.occurred_at`, claimLease.String()).Scan(
		claimLease.Milliseconds(), &event.ID, &event.Type, &event.AggregateID, &event.TenantID, &event.Payload, &event.OccurredAt,
	)
	if err == pgx.ErrNoRows {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, commitErr
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &event, nil
}

func (s Store) Complete(ctx context.Context, id int64) error {
	_, err := s.Pool.Exec(ctx, `UPDATE credit_outbox SET published_at=now(), publishing_at=NULL, last_error=NULL WHERE id=$1 AND published_at IS NULL`, id)
	return err
}

func (s Store) Fail(ctx context.Context, id int64, cause string, retryAfter time.Duration) error {
	_, err := s.Pool.Exec(ctx, `UPDATE credit_outbox SET publishing_at=NULL, available_at=now()+($2 * interval '1 millisecond'), last_error=$3 WHERE id=$1 AND published_at IS NULL`, id, retryAfter.Milliseconds(), cause)
	return err
}
