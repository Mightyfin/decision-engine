package creditrisk

import (
	"context"
	"github.com/Mightyfin/decision-engine/pricing"
	"github.com/Mightyfin/decision-engine/product"
	"testing"
	"time"
)

type products struct{ p product.Policy }

func (m products) Policy(context.Context, string, string) (product.Policy, error) { return m.p, nil }

type memory struct {
	a     map[string]Application
	o     map[string]Offer
	audit []Audit
}

func (m *memory) Application(_ context.Context, id string) (Application, error) {
	a, ok := m.a[id]
	if !ok {
		return Application{}, ErrNotFound
	}
	return a, nil
}
func (m *memory) ReviewQueue(_ context.Context, tenantID string, limit int) ([]Application, error) {
	items := make([]Application, 0, limit)
	for _, a := range m.a {
		if a.TenantID == tenantID && a.Status == "pending_review" && len(items) < limit {
			items = append(items, a)
		}
	}
	return items, nil
}
func (m *memory) SaveApplication(_ context.Context, a Application) error { m.a[a.ID] = a; return nil }
func (m *memory) SaveOffer(_ context.Context, o Offer) error             { m.o[o.ApplicationID] = o; return nil }
func (m *memory) Offer(_ context.Context, id string) (Offer, error) {
	o, ok := m.o[id]
	if !ok {
		return Offer{}, ErrNotFound
	}
	return o, nil
}
func (m *memory) Exposure(context.Context, string, string) (Exposure, error) { return Exposure{}, nil }
func (m *memory) AppendAudit(_ context.Context, a Audit) error {
	m.audit = append(m.audit, a)
	return nil
}
func TestManualDecisionLifecycle(t *testing.T) {
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	m := &memory{a: map[string]Application{}, o: map[string]Offer{}}
	s := Service{Store: m, Products: product.Service{Store: products{product.Policy{ID: "p", TenantID: "t", Currency: "ZMW", Version: 2, Active: true, MinimumAmount: 100, MaximumAmount: 10_000, MinimumTermDays: 7, MaximumTermDays: 90}}}, Pricing: pricing.QuoteFor, Clock: func() time.Time { return now }}
	a, e := s.Submit(context.Background(), Application{ID: "app", TenantID: "t", ProductPolicyID: "p", RelationshipID: "rel", Currency: "ZMW", Purpose: "stock", Amount: 1000, TermDays: 30}, "partner")
	if e != nil || a.Status != "pending_review" || a.ProductPolicyVersion != 2 {
		t.Fatal(a, e)
	}
	queue, e := m.ReviewQueue(context.Background(), "t", 10)
	if e != nil || len(queue) != 1 || queue[0].ID != "app" {
		t.Fatal(queue, e)
	}
	a, e = s.Decide(context.Background(), "app", "reviewer", "offer", "verified trading history", pricing.Policy{ProductPolicyID: "p", ProductPolicyVersion: 2, Version: 3, AnnualRateBPS: 1200, OriginationFeeBPS: 100, Currency: "ZMW", Active: true})
	if e != nil || a.Status != "offered" {
		t.Fatal(a, e)
	}
	a, e = s.Accept(context.Background(), "app", "partner")
	if e != nil || a.Status != "accepted" || len(m.audit) != 3 {
		t.Fatal(a, e, len(m.audit))
	}
}
