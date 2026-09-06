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
