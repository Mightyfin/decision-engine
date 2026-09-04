package product

import (
	"context"
	"testing"
)

type testStore struct{ policy Policy }

func (s testStore) Policy(context.Context, string, string) (Policy, error) { return s.policy, nil }

func TestValidateUsesConfiguredPolicy(t *testing.T) {
	s := Service{Store: testStore{Policy{ID: "prd", TenantID: "ten", Currency: "ZMW", Version: 2, Active: true, MinimumAmount: 100, MaximumAmount: 1000, MinimumTermDays: 7, MaximumTermDays: 30}}}
	if _, err := s.Validate(context.Background(), "ten", "prd", "ZMW", 500, 14); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Validate(context.Background(), "ten", "prd", "ZMW", 1_001, 14); err == nil {
		t.Fatal("expected configured amount bound")
	}
}
