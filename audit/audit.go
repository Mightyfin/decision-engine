// Package audit owns the append-only audit contract used by decision domains.
package audit

import (
	"context"
	"time"
)

type Record struct {
	AggregateID, Actor, Action, Reason string
	PolicyVersions                     map[string]int
	At                                 time.Time
}

// Store implementations must reject update and delete operations for stored records.
type Store interface {
	Append(context.Context, Record) error
}
