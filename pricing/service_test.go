package pricing

import (
	"testing"
	"time"
)

func TestQuoteCapturesVersionedPolicy(t *testing.T) {
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	q, err := QuoteFor(Policy{ProductPolicyID: "prd_stock", ProductPolicyVersion: 2, Version: 4, AnnualRateBPS: 1200, OriginationFeeBPS: 100, PenaltyRateBPS: 2500, PenaltyBasis: "overdue_principal", PenaltyCapBPS: 10000, Currency: "ZMW", Active: true}, ScheduleTerms{RepaymentIntervalDays: 30, GraceDays: 3, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}}, 100_000, 90, now)
	if err != nil || q.ProductPolicyVersion != 2 || q.PricingPolicyVersion != 4 || q.Total != q.Principal+q.Interest+q.Fees || q.InstallmentCount != 3 || q.GraceDays != 3 || q.PenaltyRateBPS != 2500 {
		t.Fatal(q, err)
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
