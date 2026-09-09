// Package httpapi exposes the Decision Engine through an authenticated adapter.
package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/pricing"
	"github.com/Mightyfin/decision-engine/product"
)

type Principal struct {
	Subject, TenantID string
	ApplicationID     string
	Environment       string
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
type policyStore interface {
	CreatePricingPolicy(context.Context, string, pricing.Policy) error
}
type Server struct {
	DocumentURL  string
	Auth         Authenticator
	Credit       creditrisk.Service
	Applications applicationStore
	Pricing      pricingStore
	Policies     policyStore
}

func (s Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		write(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	m.HandleFunc("POST /v1/credit/applications", s.submit)
	m.HandleFunc("POST /v1/credit/applications/{id}/cancel", s.cancelApplication)
	m.HandleFunc("GET /v1/credit/applications/{id}/commercial-review", s.commercialReview)
	m.HandleFunc("POST /v1/credit/applications/{id}/commercial-review", s.commercialReview)
	m.HandleFunc("GET /v1/credit/applications", s.listApplications)
	m.HandleFunc("POST /v1/credit/application-drafts", s.submit)
	m.HandleFunc("GET /v1/credit/applications/{id}/draft", s.draft)
	m.HandleFunc("PUT /v1/credit/applications/{id}/draft", s.draft)
	m.HandleFunc("POST /v1/credit/applications/{id}/submit", s.draft)
	m.HandleFunc("POST /v1/credit/applications/{id}/evidence", s.bindEvidence)
	m.HandleFunc("POST /v1/credit/applications/{id}/document-access", s.documentAccess)
	m.HandleFunc("GET /v1/credit/applications/{id}/information-request", s.information)
	m.HandleFunc("POST /v1/credit/applications/{id}/resubmit", s.information)
	m.HandleFunc("POST /v1/internal/tenants/{tenant_id}/credit/applications/{id}/information-request", s.information)
	m.HandleFunc("GET /v1/credit/applications/{id}", s.get)
	m.HandleFunc("GET /v1/credit/applications/{id}/offer", s.getOffer)
	m.HandleFunc("GET /v1/credit/applications/{id}/history", s.tenantTimeline)
	m.HandleFunc("POST /v1/credit/applications/{id}/accept", s.accept)
	m.HandleFunc("GET /v1/internal/tenants/{tenant_id}/credit/review-queue", s.queue)
	m.HandleFunc("GET /v1/internal/tenants/{tenant_id}/credit/review-queue/page", s.queuePage)
	m.HandleFunc("GET /v1/internal/tenants/{tenant_id}/credit/applications/{id}/review", s.reviewCase)
	m.HandleFunc("GET /v1/internal/tenants/{tenant_id}/credit/applications/{id}/evidence", s.listEvidence)
	m.HandleFunc("POST /v1/internal/credit/applications/{id}/decision", s.decide)
	m.HandleFunc("POST /v1/internal/tenants/{tenant_id}/credit/pricing-policies", s.createPricingPolicy)
	return m
}

func (s Server) createPricingPolicy(w http.ResponseWriter, r *http.Request) {
	_, ok := s.principal(w, r, "credit_policy_admin")
	if !ok {
		return
	}
	if s.Policies == nil {
		write(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var in struct {
		ProductPolicyID   string `json:"product_policy_id"`
		Version           int    `json:"version"`
		AnnualRateBPS     int    `json:"annual_rate_bps"`
		InterestMethod    string `json:"interest_method"`
		RatePeriod        string `json:"rate_period"`
		InterestRateBPS   int    `json:"interest_rate_bps"`
		FixedInterest     int64  `json:"fixed_interest_minor"`
		OriginationFeeBPS int    `json:"origination_fee_bps"`
		PenaltyRateBPS    int    `json:"penalty_rate_bps"`
		PenaltyBasis      string `json:"penalty_basis"`
		PenaltyCapBPS     int    `json:"penalty_cap_bps"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		write(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if in.InterestMethod == "" {
		in.InterestMethod = "flat"
	}
	if in.RatePeriod == "" {
		in.RatePeriod = "annual"
	}
	if in.InterestRateBPS == 0 {
		in.InterestRateBPS = in.AnnualRateBPS
	}
	if strings.TrimSpace(in.ProductPolicyID) == "" || in.Version < 1 || in.AnnualRateBPS < 0 || in.InterestRateBPS < 0 || in.FixedInterest < 0 || in.OriginationFeeBPS < 0 || in.PenaltyRateBPS < 0 || in.PenaltyCapBPS < 0 || strings.TrimSpace(in.PenaltyBasis) == "" {
		write(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if err := s.Policies.CreatePricingPolicy(r.Context(), r.PathValue("tenant_id"), pricing.Policy{ProductPolicyID: in.ProductPolicyID, Version: in.Version, AnnualRateBPS: in.AnnualRateBPS, InterestMethod: in.InterestMethod, RatePeriod: in.RatePeriod, InterestRateBPS: in.InterestRateBPS, FixedInterest: in.FixedInterest, OriginationFeeBPS: in.OriginationFeeBPS, PenaltyRateBPS: in.PenaltyRateBPS, PenaltyBasis: in.PenaltyBasis, PenaltyCapBPS: in.PenaltyCapBPS, Active: true}); err != nil {
		write(w, 422, map[string]string{"error": "policy_rejected"})
		return
	}
	write(w, 201, map[string]any{"product_policy_id": in.ProductPolicyID, "version": in.Version})
}
func (s Server) principal(w http.ResponseWriter, r *http.Request, role string) (Principal, bool) {
	p, err := s.Auth.Authenticate(r)
	if err != nil || !p.Roles[role] {
		write(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return p, false
	}
	if p.ApplicationID != "" && r.PathValue("id") != "" {
		if guard, ok := s.Applications.(interface {
			CheckDraftCaller(context.Context, string, string, string, string) error
		}); ok {
			if err := guard.CheckDraftCaller(r.Context(), r.PathValue("id"), p.TenantID, p.Environment, p.ApplicationID); err != nil {
				if errors.Is(err, creditrisk.ErrNotFound) {
					write(w, 404, map[string]string{"error": "not_found"})
				} else {
					write(w, 503, map[string]string{"error": "application_unavailable"})
				}
				return p, false
			}
		}
	}
	return p, true
}

func (s Server) submit(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "credit_application_writer")
	if !ok || p.TenantID == "" {
		if ok {
			write(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		}
		return
	}
	var in struct {
		ProductPolicyID string `json:"product_policy_id"`
		RelationshipID  string `json:"relationship_id"`
		PartyID         string `json:"party_id"`
		ApplicantRole   string `json:"applicant_role"`
		WalletID        string `json:"wallet_id"`
		Origin          string `json:"origin"`
		Currency        string `json:"currency"`
		Purpose         string `json:"purpose"`
		Amount          int64  `json:"amount_minor"`
		TermDays        int    `json:"term_days"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		write(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 16 || len(key) > 128 || p.ApplicationID == "" || p.Environment == "" || p.Subject == "" {
		write(w, 400, map[string]string{"error": "application_identity_and_idempotency_key_required"})
		return
	}
	encoded, _ := json.Marshal(in)
	requestHash := sha256.Sum256(encoded)
	ctx := product.WithBearerToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	input := creditrisk.Application{ID: newID("cap"), Environment: p.Environment, TenantID: p.TenantID, ProductPolicyID: in.ProductPolicyID, RelationshipID: in.RelationshipID, PartyID: strings.TrimSpace(in.PartyID), ApplicantRole: strings.TrimSpace(in.ApplicantRole), WalletID: strings.TrimSpace(in.WalletID), Origin: strings.TrimSpace(in.Origin), Currency: in.Currency, Purpose: in.Purpose, Amount: in.Amount, TermDays: in.TermDays}
	if r.URL.Path == "/v1/credit/application-drafts" {
		if !p.Roles["credit_application_writer"] {
			write(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" || len(key) > 128 || p.ApplicationID == "" {
			write(w, 400, map[string]string{"error": "application_identity_and_idempotency_key_required"})
			return
		}
		encoded, _ := json.Marshal(in)
		hash := sha256.Sum256(encoded)
		scope, _ := json.Marshal([]string{p.TenantID, p.Environment, p.ApplicationID, key})
		identity := sha256.Sum256(scope)
		input.ID = "cap_" + hex.EncodeToString(identity[:])
		ctx = creditrisk.WithDraftCreation(ctx, p.ApplicationID, hex.EncodeToString(hash[:]))
		d, err := s.Credit.CreateDraft(ctx, input, p.Subject)
		if err != nil {
			draftError(w, err)
			return
		}
		write(w, 201, d)
		return
	}
	ctx = creditrisk.WithSubmissionIdentity(ctx, creditrisk.SubmissionIdentity{TenantID: p.TenantID, Environment: p.Environment, CallerApplicationID: p.ApplicationID, Key: key, Hash: hex.EncodeToString(requestHash[:])})
	a, err := s.Credit.Submit(ctx, input, p.Subject)
	if err != nil {
		draftError(w, err)
		return
	}
	write(w, 202, a)
}

func (s Server) get(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "decision_workload")
	if !ok {
		return
	}
	a, ok := s.tenantApplication(w, r, p)
	if !ok {
		return
	}
	write(w, 200, a)
}

// accept records a tenant's acceptance of an existing offer. It does not
// reserve money, disburse, create an LMS loan, or post to a ledger.
func (s Server) accept(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "credit_application_writer")
	if !ok {
		return
	}
	a, ok := s.tenantApplication(w, r, p)
	if !ok {
		return
	}
	var input struct {
		QuoteID          string `json:"quote_id"`
		ConsentReference string `json:"consent_reference"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF || len(key) < 16 || len(key) > 128 || strings.TrimSpace(input.QuoteID) == "" || len(input.QuoteID) > 128 || strings.TrimSpace(input.ConsentReference) == "" || len(input.ConsentReference) > 256 || p.ApplicationID == "" {
		write(w, 400, map[string]string{"error": "quote_consent_application_and_idempotency_key_required"})
		return
	}
	a, replayed, err := s.Credit.AcceptIdempotent(r.Context(), a, p.Subject, creditrisk.AcceptanceRequest{TenantID: p.TenantID, Environment: p.Environment, CallerApplicationID: p.ApplicationID, Key: key, QuoteID: input.QuoteID, ConsentReference: input.ConsentReference})
	if err != nil {
		status, code := 503, "acceptance_unavailable"
		if errors.Is(err, creditrisk.ErrInvalidState) {
			status, code = 409, "offer_state_or_quote_conflict"
		}
		if errors.Is(err, creditrisk.ErrAcceptanceConflict) {
			status, code = 409, "idempotency_conflict"
		}
		if errors.Is(err, creditrisk.ErrNotFound) {
			status, code = 404, "not_found"
		}
		write(w, status, map[string]string{"error": code})
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	write(w, http.StatusOK, a)
}

func (s Server) queue(w http.ResponseWriter, r *http.Request) {
	_, ok := s.principal(w, r, "credit_analyst")
	if !ok {
		return
	}
	tenant := r.PathValue("tenant_id")
	queue, err := s.Applications.ReviewQueue(r.Context(), tenant, 50)
	if err != nil {
		write(w, 500, map[string]string{"error": "internal_error"})
		return
	}
	write(w, 200, queue)
}

func (s Server) queuePage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.principal(w, r, "credit_analyst"); !ok {
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			write(w, 400, map[string]string{"error": "invalid_limit"})
			return
		}
	}
	store, ok := s.Applications.(interface {
		ReviewQueuePage(context.Context, string, int, string) ([]creditrisk.Application, error)
	})
	if !ok {
		write(w, 503, map[string]string{"error": "pagination_unavailable"})
		return
	}
	items, err := store.ReviewQueuePage(r.Context(), r.PathValue("tenant_id"), limit+1, r.URL.Query().Get("after"))
	if err != nil {
		write(w, 500, map[string]string{"error": "internal_error"})
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = items[len(items)-1].ID
	}
	if items == nil {
		items = []creditrisk.Application{}
	}
	write(w, 200, map[string]any{"data": items, "next_cursor": next})
}

func (s Server) decide(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "credit_analyst")
	if !ok {
		return
	}
	a, err := s.Applications.Application(r.Context(), r.PathValue("id"))
	if err != nil {
		write(w, 404, map[string]string{"error": "not_found"})
		return
	}
	var in struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
		Revision string `json:"review_revision"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Reason) == "" || in.Revision == "" {
		write(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	policy, err := s.Pricing.PricingPolicy(r.Context(), a.TenantID, a.ProductPolicyID)
	if err != nil {
		write(w, 422, map[string]string{"error": "pricing_policy_unavailable"})
		return
	}
	policy.ProductPolicyVersion = a.ProductPolicyVersion
	policy.Currency = a.Currency
	a, err = s.Credit.Decide(creditrisk.WithReviewVersion(r.Context(), in.Revision), a.ID, p.Subject, in.Decision, in.Reason, policy)
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
