package pricing

import (
	"fmt"
	"time"
)

// Policy belongs to the Pricing Engine. Rates are policy data, not approval logic.
type Policy struct {
	ProductPolicyID                                                 string
	ProductPolicyVersion, Version, AnnualRateBPS, OriginationFeeBPS int
	Currency                                                        string
	Active                                                          bool
}
type Quote struct {
	ID, ProductPolicyID, Currency              string
	ProductPolicyVersion, PricingPolicyVersion int
	Principal, Interest, Fees, Total           int64
	ExpiresAt                                  time.Time
}

func QuoteFor(p Policy, principal int64, termDays int, now time.Time) (Quote, error) {
	if !p.Active || p.ProductPolicyVersion < 1 || p.Version < 1 || principal <= 0 || termDays <= 0 || p.AnnualRateBPS < 0 || p.OriginationFeeBPS < 0 {
		return Quote{}, fmt.Errorf("invalid configured pricing policy")
	}
	// Integer arithmetic makes the quote reproducible. This is a disclosure quote, not an LMS schedule.
	interest := principal * int64(p.AnnualRateBPS) * int64(termDays) / (10_000 * 365)
	fees := principal * int64(p.OriginationFeeBPS) / 10_000
	return Quote{ID: fmt.Sprintf("qte_%d", now.UnixNano()), ProductPolicyID: p.ProductPolicyID, Currency: p.Currency, ProductPolicyVersion: p.ProductPolicyVersion, PricingPolicyVersion: p.Version, Principal: principal, Interest: interest, Fees: fees, Total: principal + interest + fees, ExpiresAt: now.UTC().Add(24 * time.Hour)}, nil
}
