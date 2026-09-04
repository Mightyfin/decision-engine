// Package httpapi exposes the Decision Engine through an authenticated adapter.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/pricing"
)

type Principal struct {
	Subject, TenantID string
	Roles             map[string]bool
}

type Authenticator interface {
	Authenticate(*http.Request) (Principal, error)
}
type pricingStore interface {
	PricingPolicy(context.Context, string, string) (pricing.Policy, error)
}
type applicationStore interface {
	Application(context.Context, string) (creditrisk.Application, error)
	ReviewQueue(context.Context, string, int) ([]creditrisk.Application, error)
}
type Server struct {
	Auth         Authenticator
	Credit       creditrisk.Service
	Applications applicationStore
	Pricing      pricingStore
}

func (s Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("POST /v1/credit/applications", s.submit)
	m.HandleFunc("GET /v1/credit/applications/{id}", s.get)
	m.HandleFunc("GET /v1/internal/tenants/{tenant_id}/credit/review-queue", s.queue)
	m.HandleFunc("POST /v1/internal/credit/applications/{id}/decision", s.decide)
	return m
}
func (s Server) principal(w http.ResponseWriter, r *http.Request, role string) (Principal, bool) {
	p, err := s.Auth.Authenticate(r)
	if err != nil || p.TenantID == "" || !p.Roles[role] {
		write(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return p, false
	}
	return p, true
}

func (s Server) submit(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "decision_workload")
	if !ok {
		return
	}
	var in struct {
		ProductPolicyID string `json:"product_policy_id"`
		RelationshipID  string `json:"relationship_id"`
		Currency        string `json:"currency"`
		Purpose         string `json:"purpose"`
		Amount          int64  `json:"amount_minor"`
		TermDays        int    `json:"term_days"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		write(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	a, err := s.Credit.Submit(r.Context(), creditrisk.Application{ID: newID("cap"), TenantID: p.TenantID, ProductPolicyID: in.ProductPolicyID, RelationshipID: in.RelationshipID, Currency: in.Currency, Purpose: in.Purpose, Amount: in.Amount, TermDays: in.TermDays}, p.Subject)
	if err != nil {
		write(w, 422, map[string]string{"error": "validation_failed"})
		return
	}
	write(w, 202, a)
}

func (s Server) get(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "decision_workload")
	if !ok {
		return
	}
	a, err := s.Applications.Application(r.Context(), r.PathValue("id"))
	if err != nil || a.TenantID != p.TenantID {
		write(w, 404, map[string]string{"error": "not_found"})
		return
	}
	write(w, 200, a)
}

func (s Server) queue(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "credit_analyst")
	if !ok {
		return
	}
	tenant := r.PathValue("tenant_id")
	if tenant != p.TenantID {
		write(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	queue, err := s.Applications.ReviewQueue(r.Context(), tenant, 50)
	if err != nil {
		write(w, 500, map[string]string{"error": "internal_error"})
		return
	}
	write(w, 200, queue)
}

func (s Server) decide(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "credit_analyst")
	if !ok {
		return
	}
	a, err := s.Applications.Application(r.Context(), r.PathValue("id"))
	if err != nil || a.TenantID != p.TenantID {
		write(w, 404, map[string]string{"error": "not_found"})
		return
	}
	var in struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Reason) == "" {
		write(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	policy, err := s.Pricing.PricingPolicy(r.Context(), p.TenantID, a.ProductPolicyID)
	if err != nil {
		write(w, 422, map[string]string{"error": "pricing_policy_unavailable"})
		return
	}
	a, err = s.Credit.Decide(r.Context(), a.ID, p.Subject, in.Decision, in.Reason, policy)
	if err != nil {
		write(w, 422, map[string]string{"error": "decision_rejected"})
		return
	}
	write(w, 200, a)
}

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func newID(prefix string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
