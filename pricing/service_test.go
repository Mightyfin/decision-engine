package pricing

import (
	"testing"
	"time"
)

func TestQuoteCapturesVersionedPolicy(t *testing.T) {
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	q, err := QuoteFor(Policy{ProductPolicyID: "prd_stock", ProductPolicyVersion: 2, Version: 4, AnnualRateBPS: 1200, OriginationFeeBPS: 100, PenaltyRateBPS: 2500, PenaltyBasis: "overdue_principal", PenaltyCapBPS: 10000, Currency: "ZMW", Active: true}, ScheduleTerms{RepaymentIntervalDays: 30, GraceDays: 3, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}}, 100_000, 90, now)
	if err != nil || q.ProductPolicyVersion != 2 || q.PricingPolicyVersion != 4 || q.Total != q.Principal+q.Interest+q.Fees || q.InstallmentCount != 3 || q.GraceDays != 3 || q.PenaltyRateBPS != 2500 || q.InterestMethod != "flat" || q.RatePeriod != "annual" || q.InterestRateBPS != 1200 || len(q.ChargeLines) != 2 || q.ChargeLines[1].Code != "origination_fee" {
		t.Fatal(q, err)
	}
}

func TestQuoteRejectsMoneyOverflow(t *testing.T) {
	_, err := QuoteFor(Policy{ProductPolicyID: "prd", ProductPolicyVersion: 1, Version: 1, InterestMethod: "flat", RatePeriod: "term", InterestRateBPS: 10_000, PenaltyBasis: "overdue_principal", Currency: "ZMW", Active: true}, ScheduleTerms{RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}}, int64(^uint64(0)>>1), 30, time.Now())
	if err == nil {
		t.Fatal("expected an overflow error")
	}
}

func TestQuoteCalculatesItemisedBorrowerCharges(t *testing.T) {
	p := Policy{ProductPolicyID: "prd", ProductPolicyVersion: 1, Version: 1, InterestMethod: "flat", RatePeriod: "term", InterestRateBPS: 1000, OriginationFeeBPS: 100, BorrowerCharges: []BorrowerChargePolicy{{Code: "management_fee", Method: "percentage_of_principal", RateBPS: 200}, {Code: "communication_fee", Method: "fixed_amount", FixedAmount: 5000}}, PenaltyBasis: "overdue_principal", Currency: "ZMW", Active: true}
	q, err := QuoteFor(p, ScheduleTerms{RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}}, 100_000, 30, time.Now())
	if err != nil || q.Interest != 10_000 || q.Fees != 8_000 || q.Total != 118_000 || len(q.ChargeLines) != 4 || q.ChargeLines[2].Code != "management_fee" || q.ChargeLines[3].Amount != 5_000 {
		t.Fatalf("quote=%+v err=%v", q, err)
	}
}

func TestBorrowerChargesRejectDuplicateOrAmbiguousDefinitions(t *testing.T) {
	for _, charges := range [][]BorrowerChargePolicy{
		{{Code: "interest", Method: "fixed_amount", FixedAmount: 1}},
		{{Code: "service_fee", Method: "fixed_amount", FixedAmount: 1}, {Code: "service_fee", Method: "fixed_amount", FixedAmount: 2}},
		{{Code: "service_fee", Method: "percentage_of_principal", RateBPS: 100, FixedAmount: 1}},
	} {
		if err := ValidateBorrowerCharges(Policy{BorrowerCharges: charges}); err == nil {
			t.Fatalf("accepted invalid charges: %+v", charges)
		}
	}
}

func TestMonthlyFlatAndReducingBalance(t *testing.T) {
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	terms := ScheduleTerms{RepaymentIntervalDays: 30, GraceDays: 3, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}}
	base := Policy{ProductPolicyID: "prd", ProductPolicyVersion: 1, Version: 1, InterestMethod: "flat", RatePeriod: "monthly", InterestRateBPS: 1600, PenaltyBasis: "overdue_principal", Currency: "ZMW", Active: true}
	flat, e := QuoteFor(base, terms, 500_000, 90, now)
	if e != nil || flat.Interest != 240_000 || flat.Total != 740_000 {
		t.Fatalf("flat=%+v err=%v", flat, e)
	}
	base.InterestMethod = "reducing_balance"
	reducing, e := QuoteFor(base, terms, 500_000, 90, now)
	if e != nil || reducing.Interest != 160_000 {
		t.Fatalf("reducing=%+v err=%v", reducing, e)
	}
}
