package pricing

import (
	"fmt"
	"time"
)

// Policy belongs to the Pricing Engine. Rates are policy data, not approval logic.
type Policy struct {
	ProductPolicyID                                                 string
	ProductPolicyVersion, Version, AnnualRateBPS, OriginationFeeBPS int
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
	if !p.Active || p.ProductPolicyVersion < 1 || p.Version < 1 || principal <= 0 || termDays <= 0 || p.AnnualRateBPS < 0 || p.OriginationFeeBPS < 0 || p.PenaltyRateBPS < 0 || p.PenaltyCapBPS < 0 || terms.RepaymentIntervalDays < 1 || terms.GraceDays < 0 || !validAllocationOrder(terms.AllocationOrder) || !validPenaltyBasis(p.PenaltyBasis) {
		return Quote{}, fmt.Errorf("invalid configured pricing policy")
	}
	// Integer arithmetic makes the quote reproducible. This is a disclosure quote, not an LMS schedule.
	interest := principal * int64(p.AnnualRateBPS) * int64(termDays) / (10_000 * 365)
	fees := principal * int64(p.OriginationFeeBPS) / 10_000
	installments := (termDays + terms.RepaymentIntervalDays - 1) / terms.RepaymentIntervalDays
	return Quote{ID: fmt.Sprintf("qte_%d", now.UnixNano()), ProductPolicyID: p.ProductPolicyID, Currency: p.Currency, ProductPolicyVersion: p.ProductPolicyVersion, PricingPolicyVersion: p.Version, Principal: principal, Interest: interest, Fees: fees, Total: principal + interest + fees, InstallmentCount: installments, RepaymentIntervalDays: terms.RepaymentIntervalDays, GraceDays: terms.GraceDays, PenaltyRateBPS: p.PenaltyRateBPS, PenaltyCapBPS: p.PenaltyCapBPS, PenaltyBasis: p.PenaltyBasis, AllocationOrder: append([]string(nil), terms.AllocationOrder...), ExpiresAt: now.UTC().Add(24 * time.Hour)}, nil
}

func validPenaltyBasis(value string) bool {
	switch value {
	case "overdue_principal", "overdue_interest", "overdue_principal_interest", "overdue_principal_interest_fees":
		return true
	default:
		return false
	}
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
