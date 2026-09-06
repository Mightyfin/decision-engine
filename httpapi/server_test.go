package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/pricing"
	"github.com/Mightyfin/decision-engine/product"
)

type testAuth struct{ principal Principal }

func (a testAuth) Authenticate(*http.Request) (Principal, error) { return a.principal, nil }

type testStore struct {
	applications map[string]creditrisk.Application
	offers       map[string]creditrisk.Offer
	audits       []creditrisk.Audit
}

func (s *testStore) Application(_ context.Context, id string) (creditrisk.Application, error) {
	a, ok := s.applications[id]
	if !ok {
		return creditrisk.Application{}, creditrisk.ErrNotFound
	}
	return a, nil
}
func (s *testStore) ReviewQueue(_ context.Context, tenant string, _ int) ([]creditrisk.Application, error) {
	out := []creditrisk.Application{}
	for _, a := range s.applications {
		if a.TenantID == tenant && a.Status == "pending_review" {
			out = append(out, a)
		}
	}
	return out, nil
}
func (s *testStore) SaveApplication(_ context.Context, a creditrisk.Application) error {
	s.applications[a.ID] = a
	return nil
}
func (s *testStore) SaveOffer(_ context.Context, o creditrisk.Offer) error {
	s.offers[o.ApplicationID] = o
	return nil
}
func (s *testStore) Offer(_ context.Context, id string) (creditrisk.Offer, error) {
	o, ok := s.offers[id]
	if !ok {
		return creditrisk.Offer{}, creditrisk.ErrNotFound
	}
	return o, nil
}
func (s *testStore) Exposure(context.Context, string, string) (creditrisk.Exposure, error) {
	return creditrisk.Exposure{}, nil
}
func (s *testStore) AppendAudit(_ context.Context, a creditrisk.Audit) error {
	s.audits = append(s.audits, a)
	return nil
}

type testProducts struct{ policy product.Policy }

func (p testProducts) Policy(context.Context, string, string) (product.Policy, error) {
	return p.policy, nil
}

type testPricing struct{ policy pricing.Policy }

func (p testPricing) PricingPolicy(context.Context, string, string) (pricing.Policy, error) {
	return p.policy, nil
}

func TestSubmitRequiresWorkloadRoleAndUsesMinorUnits(t *testing.T) {
	store := &testStore{applications: map[string]creditrisk.Application{}, offers: map[string]creditrisk.Offer{}}
	policy := product.Policy{ID: "product_1", TenantID: "tenant_1", Currency: "ZMW", Version: 1, Active: true, MinimumAmount: 100, MaximumAmount: 10_000, MinimumTermDays: 7, MaximumTermDays: 90}
	service := creditrisk.Service{Store: store, Products: product.Service{Store: testProducts{policy: policy}}, Pricing: pricing.QuoteFor, Clock: func() time.Time { return time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC) }}
	server := Server{Auth: testAuth{Principal{Subject: "workload", TenantID: "tenant_1", Roles: map[string]bool{"decision_workload": true}}}, Credit: service, Applications: store, Pricing: testPricing{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/credit/applications", strings.NewReader(`{"product_policy_id":"product_1","relationship_id":"customer_1","currency":"ZMW","purpose":"inventory","amount_minor":1000,"term_days":30}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusAccepted {
		t.Fatalf("got status %d: %s", response.Code, response.Body.String())
	}
	for _, application := range store.applications {
		if application.Amount != 1000 || application.Status != "pending_review" {
			t.Fatalf("unexpected application: %#v", application)
		}
		return
	}
	t.Fatal("application was not saved")
}

func TestAnalystRoleIsRequiredForDecision(t *testing.T) {
	server := Server{Auth: testAuth{Principal{Subject: "workload", TenantID: "tenant_1", Roles: map[string]bool{"decision_workload": true}}}}
	req := httptest.NewRequest(http.MethodPost, "/v1/internal/credit/applications/cap_1/decision", strings.NewReader(`{"decision":"decline","reason":"not verified"}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("got %d", response.Code)
	}
}

func TestStaffAnalystCanReviewAnotherTenantQueue(t *testing.T) {
	store := &testStore{applications: map[string]creditrisk.Application{
		"cap_1": {ID: "cap_1", TenantID: "tenant_2", Status: "pending_review"},
	}, offers: map[string]creditrisk.Offer{}}
	server := Server{Auth: testAuth{Principal{Subject: "analyst", Roles: map[string]bool{"credit_analyst": true}}}, Applications: store}
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/tenants/tenant_2/credit/review-queue", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("got status %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "cap_1") {
		t.Fatalf("expected tenant queue item, got %s", response.Body.String())
	}
}
