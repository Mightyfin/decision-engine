package creditrisk

import "errors"

var ErrInvalidCursor = errors.New("invalid application cursor")

// ApplicationListScope is derived from verified identity, never request filters.
type ApplicationListScope struct {
	TenantID, Environment, CallerApplicationID string
}

type ApplicationListFilter struct {
	Limit                          int
	Cursor, Status, RelationshipID string
}
