package product

import (
	"context"
	"errors"
	"testing"
)

type testStore struct{ policy Policy }

func (s testStore) Policy(context.Context, string, string) (Policy, error) { return s.policy, nil }

func TestValidateUsesConfiguredPolicy(t *testing.T) {
	s := Service{Store: testStore{Policy{ID: "prd", TenantID: "ten", Currency: "ZMW", Version: 2, Active: true, MinimumAmount: 100, MaximumAmount: 1000, MinimumTermDays: 7, MaximumTermDays: 30, RepaymentIntervalDays: 30, GraceDays: 3, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}}}}
	if _, err := s.Validate(context.Background(), "ten", "prd", "ZMW", 500, 14); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Validate(context.Background(), "ten", "prd", "ZMW", 1_001, 14); err == nil {
		t.Fatal("expected configured amount bound")
	}
}

func validPolicy() Policy {
	return Policy{ID: "prd", TenantID: "ten", Currency: "ZMW", Version: 2, Active: true,
		MinimumAmount: 100, MaximumAmount: 1000, MinimumTermDays: 7, MaximumTermDays: 30,
		RepaymentIntervalDays: 30, AllocationOrder: []string{"penalty", "fees", "interest", "principal"}}
}

func TestValidateRejectsPolicySubstitution(t *testing.T) {
	for _, tc := range []struct {
		name, tenant, product string
		mutate                func(*Policy)
	}{
		{"foreign tenant", "ten", "prd", func(p *Policy) { p.TenantID = "other" }},
		{"wrong product", "ten", "prd", func(p *Policy) { p.ID = "other" }},
		{"missing tenant", "", "prd", func(p *Policy) { p.TenantID = "" }},
		{"missing product", "ten", "", func(p *Policy) { p.ID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := validPolicy()
			tc.mutate(&p)
			got, err := (Service{Store: testStore{p}}).Validate(context.Background(), tc.tenant, tc.product, "ZMW", 500, 14)
			if !errors.Is(err, ErrNotFound) || got.ID != "" {
				t.Fatalf("substituted policy accepted or leaked: %+v %v", got, err)
			}
		})
	}
}

func TestValidateRejectsMalformedPolicy(t *testing.T) {
	for name, change := range map[string]func(*Policy){
		"zero principal minimum":     func(p *Policy) { p.MinimumAmount = 0 },
		"negative principal minimum": func(p *Policy) { p.MinimumAmount = -1 },
		"inverted amount range":      func(p *Policy) { p.MaximumAmount = 1 },
		"zero term minimum":          func(p *Policy) { p.MinimumTermDays = 0 },
		"inverted term range":        func(p *Policy) { p.MaximumTermDays = 1 },
		"empty currency":             func(p *Policy) { p.Currency = "" },
		"lowercase currency":         func(p *Policy) { p.Currency = "zmw" },
		"non-letter currency":        func(p *Policy) { p.Currency = "Z1W" },
	} {
		t.Run(name, func(t *testing.T) {
			p := validPolicy()
			change(&p)
			if got, err := (Service{Store: testStore{p}}).Validate(context.Background(), "ten", "prd", p.Currency, 500, 14); err == nil || got.ID != "" {
				t.Fatal("malformed policy accepted")
			}
		})
	}
}

func TestValidateIncludesExactBounds(t *testing.T) {
	s := Service{Store: testStore{validPolicy()}}
	for _, tc := range []struct {
		amount int64
		days   int
	}{{100, 7}, {1000, 30}} {
		if _, err := s.Validate(context.Background(), "ten", "prd", "ZMW", tc.amount, tc.days); err != nil {
			t.Fatal(err)
		}
	}
}
