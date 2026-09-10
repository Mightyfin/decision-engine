package creditrisk

import (
	"context"
	"encoding/json"
	"github.com/Mightyfin/decision-engine/pricing"
	"github.com/Mightyfin/decision-engine/product"
	"testing"
	"time"
)

func TestUsageSnapshotSurvivesProductChanges(t *testing.T) {
	const usage = `{"funding_mode":"borrower_cash","destination_rule":"borrower_wallet","allow_partial_use":false,"usage_expiry_days":0,"repayment_restoration":"none"}`
	p := product.Policy{ID: "p", TenantID: "t", Active: true, Version: 1, Currency: "ZMW", MinimumAmount: 1, MaximumAmount: 10000, MinimumTermDays: 1, MaximumTermDays: 90, RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}, UsageTerms: json.RawMessage(usage)}
	m := &memory{a: map[string]Application{}, o: map[string]Offer{}}
	s := Service{Store: m, Products: product.Service{Store: products{p}}, Pricing: pricing.QuoteFor, Clock: func() time.Time { return time.Now().UTC() }}
	a, err := s.Submit(context.Background(), Application{ID: "snapshot", TenantID: "t", ProductPolicyID: "p", RelationshipID: "r", Currency: "ZMW", Purpose: "test", Amount: 1000, TermDays: 30, UsageTerms: json.RawMessage(`{"funding_mode":"restricted_goods"}`)}, "actor")
	if err != nil || string(a.UsageTerms) != usage {
		t.Fatalf("product must override caller usage: %s %v", a.UsageTerms, err)
	}
	p.UsageTerms[0] = '!'
	s.Products = product.Service{Store: products{product.Policy{}}} // Today's policy cannot replace frozen terms.
	_, err = s.Decide(context.Background(), a.ID, "staff", "offer", "review complete", pricing.Policy{ProductPolicyID: "p", ProductPolicyVersion: 1, Version: 1, Active: true, Currency: "ZMW", PenaltyBasis: "overdue_principal"})
	if err != nil {
		t.Fatal(err)
	}
	o := m.o[a.ID]
	if string(o.UsageTerms) != usage {
		t.Fatal("offer lost original usage")
	}
	a.UsageTerms[0] = '!'
	if string(o.UsageTerms) != usage {
		t.Fatal("offer shares mutable application bytes")
	}
}
