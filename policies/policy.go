// Package policies contains versioned, explainable policy metadata shared by the financial engines.
package policies

import "time"

// Version identifies the exact configuration used for a decision or quote.
// A later policy change must create a new version rather than rewrite history.
type Version struct {
	Domain, PolicyID, TenantID string
	Number                     int
	EffectiveAt                time.Time
}
