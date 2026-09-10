package pricing

import (
	"fmt"
	"math"
	"regexp"
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
	BorrowerCharges                                                 []BorrowerChargePolicy
	Currency                                                        string
	Active                                                          bool
}

// BorrowerChargePolicy describes only charges that form part of the credit
// offer owed by the borrower. Tenant subscriptions, payment-rail charges and
// revenue sharing belong to their respective billing domains.
type BorrowerChargePolicy struct {
	Code        string `json:"code"`
	Method      string `json:"method"`
	RateBPS     int    `json:"rate_bps,omitempty"`
	FixedAmount int64  `json:"fixed_amount_minor,omitempty"`
}

var chargeCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type Quote struct {
	ID, ProductPolicyID, Currency              string
	InterestMethod, RatePeriod                 string
	ProductPolicyVersion, PricingPolicyVersion int
	InterestRateBPS                            int
	Principal, Interest, Fees, Total           int64
	InstallmentCount, RepaymentIntervalDays    int
	GraceDays, PenaltyRateBPS, PenaltyCapBPS   int
	PenaltyBasis                               string
	AllocationOrder                            []string
	ChargeLines                                []ChargeLine
	ExpiresAt                                  time.Time
}

// ChargeLine preserves how each non-principal amount in a quote was produced.
// Fees remains on Quote as a backwards-compatible aggregate, but downstream
// services should use these lines whenever they need fee identity or an audit
// explanation. Accounting treatment and beneficiaries are deliberately not
// inferred by the calculator.
type ChargeLine struct {
	Code              string `json:"code"`
	Category          string `json:"category"`
	CalculationMethod string `json:"calculation_method"`
	RatePeriod        string `json:"rate_period,omitempty"`
	TimeConvention    string `json:"time_convention,omitempty"`
	Rounding          string `json:"rounding"`
	RateBPS           int    `json:"rate_bps,omitempty"`
	Amount            int64  `json:"amount_minor"`
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
	if !p.Active || p.ProductPolicyVersion < 1 || p.Version < 1 || principal <= 0 || termDays <= 0 || rate < 0 || p.FixedInterest < 0 || p.OriginationFeeBPS < 0 || p.PenaltyRateBPS < 0 || p.PenaltyCapBPS < 0 || terms.RepaymentIntervalDays < 1 || terms.GraceDays < 0 || !validAllocationOrder(terms.AllocationOrder) || !validPenaltyBasis(p.PenaltyBasis) || ValidateBorrowerCharges(p) != nil {
		return Quote{}, fmt.Errorf("invalid configured pricing policy")
	}
	var interest int64
	var err error
	switch method {
	case "fixed_amount":
		interest = p.FixedInterest
	case "flat":
		interest, err = rateForTerm(principal, int64(rate), period, termDays)
	case "reducing_balance":
		periods := (termDays + terms.RepaymentIntervalDays - 1) / terms.RepaymentIntervalDays
		for i := 0; i < periods; i++ {
			opening := principal - principal*int64(i)/int64(periods)
			days := terms.RepaymentIntervalDays
			if remaining := termDays - i*terms.RepaymentIntervalDays; remaining < days {
				days = remaining
			}
			periodInterest, calculationErr := rateForTerm(opening, int64(rate), period, days)
			if calculationErr != nil {
				return Quote{}, calculationErr
			}
			interest, err = addChecked(interest, periodInterest)
			if err != nil {
				return Quote{}, err
			}
		}
	default:
		return Quote{}, fmt.Errorf("unsupported configured interest method")
	}
	if err != nil {
		return Quote{}, err
	}
	feeNumerator, err := multiplyChecked(principal, int64(p.OriginationFeeBPS))
	if err != nil {
		return Quote{}, err
	}
	fees := feeNumerator / 10_000
	lines := []ChargeLine{{Code: "interest", Category: "interest", CalculationMethod: method, RatePeriod: period, TimeConvention: timeConvention(method, period), Rounding: "half_up_to_minor_unit", RateBPS: rate, Amount: interest}}
	if fees > 0 {
		lines = append(lines, ChargeLine{Code: "origination_fee", Category: "fee", CalculationMethod: "percentage_of_principal", Rounding: "truncate_to_minor_unit", RateBPS: p.OriginationFeeBPS, Amount: fees})
	}
	for _, charge := range p.BorrowerCharges {
		var amount int64
		switch charge.Method {
		case "percentage_of_principal":
			numerator, calculationErr := multiplyChecked(principal, int64(charge.RateBPS))
			if calculationErr != nil {
				return Quote{}, calculationErr
			}
			amount = divideHalfUp(numerator, 10_000)
		case "fixed_amount":
			amount = charge.FixedAmount
		}
		fees, err = addChecked(fees, amount)
		if err != nil {
			return Quote{}, err
		}
		rounding := "exact_minor_unit"
		if charge.Method == "percentage_of_principal" {
			rounding = "half_up_to_minor_unit"
		}
		lines = append(lines, ChargeLine{Code: charge.Code, Category: "fee", CalculationMethod: charge.Method, Rounding: rounding, RateBPS: charge.RateBPS, Amount: amount})
	}
	subtotal, err := addChecked(principal, interest)
	if err != nil {
		return Quote{}, err
	}
	total, err := addChecked(subtotal, fees)
	if err != nil {
		return Quote{}, err
	}
	installments := (termDays + terms.RepaymentIntervalDays - 1) / terms.RepaymentIntervalDays
	return Quote{ID: fmt.Sprintf("qte_%d", now.UnixNano()), ProductPolicyID: p.ProductPolicyID, Currency: p.Currency, InterestMethod: method, RatePeriod: period, ProductPolicyVersion: p.ProductPolicyVersion, PricingPolicyVersion: p.Version, InterestRateBPS: rate, Principal: principal, Interest: interest, Fees: fees, Total: total, InstallmentCount: installments, RepaymentIntervalDays: terms.RepaymentIntervalDays, GraceDays: terms.GraceDays, PenaltyRateBPS: p.PenaltyRateBPS, PenaltyCapBPS: p.PenaltyCapBPS, PenaltyBasis: p.PenaltyBasis, AllocationOrder: append([]string(nil), terms.AllocationOrder...), ChargeLines: lines, ExpiresAt: now.UTC().Add(24 * time.Hour)}, nil
}

func timeConvention(method, period string) string {
	if method == "fixed_amount" {
		return "not_applicable"
	}
	switch period {
	case "monthly":
		return "30_days_per_month"
	case "annual":
		return "actual_365"
	case "term":
		return "whole_term"
	default:
		return "legacy"
	}
}

func ValidateBorrowerCharges(p Policy) error {
	seen := map[string]bool{"interest": true, "principal": true, "penalty": true, "origination_fee": p.OriginationFeeBPS > 0}
	for _, charge := range p.BorrowerCharges {
		if !chargeCodePattern.MatchString(charge.Code) || seen[charge.Code] || charge.RateBPS < 0 || charge.FixedAmount < 0 {
			return fmt.Errorf("invalid configured borrower charge")
		}
		seen[charge.Code] = true
		switch charge.Method {
		case "percentage_of_principal":
			if charge.RateBPS == 0 || charge.FixedAmount != 0 {
				return fmt.Errorf("invalid configured borrower charge")
			}
		case "fixed_amount":
			if charge.FixedAmount == 0 || charge.RateBPS != 0 {
				return fmt.Errorf("invalid configured borrower charge")
			}
		default:
			return fmt.Errorf("invalid configured borrower charge")
		}
	}
	return nil
}

func validPenaltyBasis(value string) bool {
	switch value {
	case "overdue_principal", "overdue_interest", "overdue_principal_interest", "overdue_principal_interest_fees", "overdue_principal_interest_penalty", "overdue_principal_interest_fees_penalty", "overdue_interest_fees", "overdue_fees", "overdue_penalty", "overdue_principal_fees", "overdue_principal_penalty", "overdue_interest_penalty", "overdue_fees_penalty", "overdue_principal_fees_penalty", "overdue_interest_fees_penalty", "total_principal_released", "total_principal_balance":
		return true
	default:
		return false
	}
}

func rateForTerm(amount, bps int64, period string, days int) (int64, error) {
	amountRate, err := multiplyChecked(amount, bps)
	if err != nil {
		return 0, err
	}
	switch period {
	case "term":
		return divideHalfUp(amountRate, 10000), nil
	case "monthly":
		numerator, err := multiplyChecked(amountRate, int64(days))
		if err != nil {
			return 0, err
		}
		return divideHalfUp(numerator, 10000*30), nil
	case "annual":
		numerator, err := multiplyChecked(amountRate, int64(days))
		if err != nil {
			return 0, err
		}
		return divideHalfUp(numerator, 10000*365), nil
	default:
		return 0, fmt.Errorf("unsupported configured rate period")
	}
}

func multiplyChecked(left, right int64) (int64, error) {
	if left < 0 || right < 0 || left != 0 && right > math.MaxInt64/left {
		return 0, fmt.Errorf("configured pricing calculation exceeds supported money range")
	}
	return left * right, nil
}

func addChecked(left, right int64) (int64, error) {
	if left < 0 || right < 0 || left > math.MaxInt64-right {
		return 0, fmt.Errorf("configured pricing calculation exceeds supported money range")
	}
	return left + right, nil
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
