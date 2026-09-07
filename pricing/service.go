package pricing

import (
	"fmt"
	"time"
)

// Policy belongs to the Pricing Engine. Rates are policy data, not approval logic.
type Policy struct {
	ProductPolicyID                                                 string
	ProductPolicyVersion, Version, AnnualRateBPS, OriginationFeeBPS int
	InterestMethod, RatePeriod                                      string
	InterestRateBPS                                                 int
	FixedInterest                                                   int64
	PenaltyRateBPS, PenaltyCapBPS                                   int
	PenaltyBasis                                                    string
	Currency                                                        string
	Active                                                          bool
}
type Quote struct {
	ID, ProductPolicyID, Currency              string
	ProductPolicyVersion, PricingPolicyVersion int
	Principal, Interest, Fees, Total           int64
	InstallmentCount, RepaymentIntervalDays    int
	GraceDays, PenaltyRateBPS, PenaltyCapBPS   int
	PenaltyBasis                               string
	AllocationOrder                            []string
	ExpiresAt                                  time.Time
}

type ScheduleTerms struct {
	RepaymentIntervalDays, GraceDays int
	AllocationOrder                  []string
}

func QuoteFor(p Policy, terms ScheduleTerms, principal int64, termDays int, now time.Time) (Quote, error) {
	method, period, rate := p.InterestMethod, p.RatePeriod, p.InterestRateBPS
	if method == "" {
		method = "flat"
	}
	if period == "" {
		period = "annual"
	}
	if rate == 0 {
		rate = p.AnnualRateBPS
	}
	if !p.Active || p.ProductPolicyVersion < 1 || p.Version < 1 || principal <= 0 || termDays <= 0 || rate < 0 || p.FixedInterest < 0 || p.OriginationFeeBPS < 0 || p.PenaltyRateBPS < 0 || p.PenaltyCapBPS < 0 || terms.RepaymentIntervalDays < 1 || terms.GraceDays < 0 || !validAllocationOrder(terms.AllocationOrder) || !validPenaltyBasis(p.PenaltyBasis) {
		return Quote{}, fmt.Errorf("invalid configured pricing policy")
	}
	var interest int64
	switch method {
	case "fixed_amount":
		interest = p.FixedInterest
	case "flat":
		interest = rateForTerm(principal, int64(rate), period, termDays)
	case "reducing_balance":
		periods := (termDays + terms.RepaymentIntervalDays - 1) / terms.RepaymentIntervalDays
		for i := 0; i < periods; i++ {
			opening := principal - principal*int64(i)/int64(periods)
			days := terms.RepaymentIntervalDays
			if remaining := termDays - i*terms.RepaymentIntervalDays; remaining < days {
				days = remaining
			}
			interest += rateForTerm(opening, int64(rate), period, days)
		}
	default:
		return Quote{}, fmt.Errorf("unsupported configured interest method")
	}
	fees := principal * int64(p.OriginationFeeBPS) / 10_000
	installments := (termDays + terms.RepaymentIntervalDays - 1) / terms.RepaymentIntervalDays
	return Quote{ID: fmt.Sprintf("qte_%d", now.UnixNano()), ProductPolicyID: p.ProductPolicyID, Currency: p.Currency, ProductPolicyVersion: p.ProductPolicyVersion, PricingPolicyVersion: p.Version, Principal: principal, Interest: interest, Fees: fees, Total: principal + interest + fees, InstallmentCount: installments, RepaymentIntervalDays: terms.RepaymentIntervalDays, GraceDays: terms.GraceDays, PenaltyRateBPS: p.PenaltyRateBPS, PenaltyCapBPS: p.PenaltyCapBPS, PenaltyBasis: p.PenaltyBasis, AllocationOrder: append([]string(nil), terms.AllocationOrder...), ExpiresAt: now.UTC().Add(24 * time.Hour)}, nil
}

func validPenaltyBasis(value string) bool {
	switch value {
	case "overdue_principal", "overdue_interest", "overdue_principal_interest", "overdue_principal_interest_fees", "overdue_principal_interest_penalty", "overdue_principal_interest_fees_penalty", "overdue_interest_fees", "overdue_fees", "overdue_penalty", "overdue_principal_fees", "overdue_principal_penalty", "overdue_interest_penalty", "overdue_fees_penalty", "overdue_principal_fees_penalty", "overdue_interest_fees_penalty", "total_principal_released", "total_principal_balance":
		return true
	default:
		return false
	}
}

func rateForTerm(amount, bps int64, period string, days int) int64 {
	switch period {
	case "term":
		return divideHalfUp(amount*bps, 10000)
	case "monthly":
		return divideHalfUp(amount*bps*int64(days), 10000*30)
	case "annual":
		return divideHalfUp(amount*bps*int64(days), 10000*365)
	default:
		return -1
	}
}
func divideHalfUp(n, d int64) int64 {
	if d <= 0 {
		return 0
	}
	return (n + d/2) / d
}

func validAllocationOrder(order []string) bool {
	if len(order) != 4 {
		return false
	}
	seen := map[string]bool{}
	for _, component := range order {
		if component != "principal" && component != "interest" && component != "fees" && component != "penalty" || seen[component] {
			return false
		}
		seen[component] = true
	}
	return true
}
