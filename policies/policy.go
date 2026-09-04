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

// CreditRouting is configuration authored and approved by credit governance. It directs an
// algorithmic result to auto-offer, auto-decline, or analyst review. No score threshold belongs
// in application code.
type CreditRouting struct {
	PolicyID, TenantID string
	Version            int
	AutoOfferMinimum   float64
	AutoDeclineMaximum float64
	Active             bool
}

func (p CreditRouting) Valid() bool {
	return p.Active && p.Version > 0 && p.PolicyID != "" && p.AutoDeclineMaximum < p.AutoOfferMinimum
}
