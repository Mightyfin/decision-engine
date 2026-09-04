package pricing

import (
	"testing"
	"time"
)

func TestQuoteCapturesVersionedPolicy(t *testing.T) {
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	q, err := QuoteFor(Policy{ProductPolicyID: "prd_stock", ProductPolicyVersion: 2, Version: 4, AnnualRateBPS: 1200, OriginationFeeBPS: 100, Currency: "ZMW", Active: true}, 100_000, 30, now)
	if err != nil || q.ProductPolicyVersion != 2 || q.PricingPolicyVersion != 4 || q.Total != q.Principal+q.Interest+q.Fees {
		t.Fatal(q, err)
	}
}
