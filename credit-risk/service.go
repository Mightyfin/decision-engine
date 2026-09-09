package creditrisk

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Mightyfin/decision-engine/pricing"
	"github.com/Mightyfin/decision-engine/product"
)

var (
	ErrNotFound     = errors.New("credit application not found")
	ErrInvalidState = errors.New("invalid credit lifecycle transition")
)

var originPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type Application struct {
	Environment           string    `json:"environment,omitempty"`
	ID                    string    `json:"id"`
	TenantID              string    `json:"tenant_id"`
	ProductPolicyID       string    `json:"product_policy_id"`
	RelationshipID        string    `json:"relationship_id"`
	PartyID               string    `json:"party_id,omitempty"`
	ApplicantRole         string    `json:"applicant_role,omitempty"`
	WalletID              string    `json:"wallet_id,omitempty"`
	Origin                string    `json:"origin"`
	Currency              string    `json:"currency"`
	Purpose               string    `json:"purpose"`
	Status                string    `json:"status"`
	Amount                int64     `json:"amount_minor"`
	TermDays              int       `json:"term_days"`
	ProductPolicyVersion  int       `json:"product_policy_version"`
	RepaymentIntervalDays int       `json:"repayment_interval_days"`
	GraceDays             int       `json:"grace_days"`
	AllocationOrder       []string  `json:"allocation_order"`
	SubmittedAt           time.Time `json:"submitted_at,omitzero"`
}
type Offer struct {
	ApplicationID         string    `json:"application_id"`
	QuoteID               string    `json:"quote_id"`
	ProductPolicyVersion  int       `json:"product_policy_version"`
	PricingPolicyVersion  int       `json:"pricing_policy_version"`
	Principal             int64     `json:"principal_minor"`
	Interest              int64     `json:"interest_minor"`
	Fees                  int64     `json:"fees_minor"`
	Total                 int64     `json:"total_minor"`
	TermDays              int       `json:"term_days"`
	InstallmentCount      int       `json:"installment_count"`
	RepaymentIntervalDays int       `json:"repayment_interval_days"`
	GraceDays             int       `json:"grace_days"`
	PenaltyRateBPS        int       `json:"penalty_rate_bps"`
	PenaltyCapBPS         int       `json:"penalty_cap_bps"`
	PenaltyBasis          string    `json:"penalty_basis"`
	AllocationOrder       []string  `json:"allocation_order"`
	ExpiresAt             time.Time `json:"expires_at"`
}
type Exposure struct {
	ApprovedLimit int64 `json:"approved_limit_minor"`
	Reserved      int64 `json:"reserved_minor"`
	Utilised      int64 `json:"utilised_minor"`
}
type Audit struct {
	ApplicationID string    `json:"application_id"`
	Actor         string    `json:"actor"`
	Action        string    `json:"action"`
	Reason        string    `json:"reason"`
	At            time.Time `json:"at"`
}
type Store interface {
	Application(context.Context, string) (Application, error)
	ReviewQueue(context.Context, string, int) ([]Application, error)
	SaveApplication(context.Context, Application) error
	SaveOffer(context.Context, Offer) error
	Offer(context.Context, string) (Offer, error)
	Exposure(context.Context, string, string) (Exposure, error)
	AppendAudit(context.Context, Audit) error
}

// AtomicStore is implemented by durable adapters. It keeps the business state
// and its corresponding audit record in one database transaction. The base
// Store remains deliberately small so in-memory adapters stay useful in tests.
type AtomicStore interface {
	CreateApplication(context.Context, Application, Audit) error
	RecordDecision(context.Context, Application, *Offer, Audit) error
	RecordAcceptance(context.Context, Application, Audit) error
}
type AcceptanceEventStore interface {
	RecordAcceptanceWithEvent(context.Context, Application, Audit, Offer) error
}
type Service struct {
	Store    Store
	Products product.Service
	Pricing  func(pricing.Policy, pricing.ScheduleTerms, int64, int, time.Time) (pricing.Quote, error)
	Clock    func() time.Time
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}
func (s Service) Submit(ctx context.Context, a Application, actor string) (Application, error) {
	a, p, err := s.prepareApplication(ctx, a)
	if err != nil {
		return Application{}, err
	}
	r := p.RequirementsFor(a.ApplicantRole)
	if len(r.Fields) > 0 || len(r.RequiredDocumentTypes) > 0 {
		return Application{}, ErrDraftRequired
	}
	a.Status = "pending_review"
	a.SubmittedAt = s.now()
	audit := Audit{ApplicationID: a.ID, Actor: actor, Action: "submitted", Reason: "application submitted", At: s.now()}
	if store, ok := s.Store.(AtomicStore); ok {
		return a, store.CreateApplication(ctx, a, audit)
	}
	if err = s.Store.SaveApplication(ctx, a); err != nil {
		return Application{}, err
	}
	return a, s.Store.AppendAudit(ctx, audit)
}

func (s Service) prepareApplication(ctx context.Context, a Application) (Application, product.Policy, error) {
	p, err := s.Products.Validate(ctx, a.TenantID, a.ProductPolicyID, a.Currency, a.Amount, a.TermDays, a.ApplicantRole)
	if err != nil {
		return Application{}, p, err
	}
	if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.RelationshipID) == "" || strings.TrimSpace(a.Purpose) == "" {
		return Application{}, p, fmt.Errorf("application identity, relationship and purpose are required")
	}
	if a.ApplicantRole != "" && a.ApplicantRole != "network_participant" && a.ApplicantRole != "partner_organisation" {
		return Application{}, p, fmt.Errorf("unsupported applicant role")
	}
	if a.PartyID != "" && a.ApplicantRole == "" {
		return Application{}, p, fmt.Errorf("party identity requires an applicant role")
	}
	if a.Origin == "" {
		a.Origin = "legacy"
	}
	if !originPattern.MatchString(a.Origin) {
		return Application{}, p, fmt.Errorf("unsupported credit origin")
	}
	a.ProductPolicyVersion = p.Version
	a.RepaymentIntervalDays = p.RepaymentIntervalDays
	a.GraceDays = p.GraceDays
	a.AllocationOrder = append([]string(nil), p.AllocationOrder...)
	return a, p, nil
}

// Decide makes no automatic lending decision. A reviewer must supply a non-empty reason.
func (s Service) Decide(ctx context.Context, id, actor, decision, reason string, policy pricing.Policy) (Application, error) {
	a, err := s.Store.Application(ctx, id)
	if err != nil {
		return Application{}, err
	}
	if a.Status != "pending_review" || strings.TrimSpace(reason) == "" {
		return Application{}, ErrInvalidState
	}
	var offer *Offer
	switch decision {
	case "decline":
		a.Status = "declined"
	case "offer":
		if policy.ProductPolicyID != a.ProductPolicyID || policy.ProductPolicyVersion != a.ProductPolicyVersion {
			return Application{}, fmt.Errorf("pricing policy does not match application product version")
		}
		q, e := s.Pricing(policy, pricing.ScheduleTerms{RepaymentIntervalDays: a.RepaymentIntervalDays, GraceDays: a.GraceDays, AllocationOrder: a.AllocationOrder}, a.Amount, a.TermDays, s.now())
		if e != nil {
			return Application{}, e
		}
		if q.Currency != a.Currency {
			return Application{}, fmt.Errorf("quote currency does not match application")
		}
		a.Status = "offered"
		offer = &Offer{ApplicationID: a.ID, QuoteID: q.ID, ProductPolicyVersion: q.ProductPolicyVersion, PricingPolicyVersion: q.PricingPolicyVersion, Principal: q.Principal, Interest: q.Interest, Fees: q.Fees, Total: q.Total, TermDays: a.TermDays, InstallmentCount: q.InstallmentCount, RepaymentIntervalDays: q.RepaymentIntervalDays, GraceDays: q.GraceDays, PenaltyRateBPS: q.PenaltyRateBPS, PenaltyCapBPS: q.PenaltyCapBPS, PenaltyBasis: q.PenaltyBasis, AllocationOrder: q.AllocationOrder, ExpiresAt: q.ExpiresAt}
	default:
		return Application{}, fmt.Errorf("unsupported manual decision")
	}
	audit := Audit{ApplicationID: a.ID, Actor: actor, Action: decision, Reason: reason, At: s.now()}
	if store, ok := s.Store.(AtomicStore); ok {
		return a, store.RecordDecision(ctx, a, offer, audit)
	}
	if offer != nil {
		if err = s.Store.SaveOffer(ctx, *offer); err != nil {
			return Application{}, err
		}
	}
	if err = s.Store.SaveApplication(ctx, a); err != nil {
		return Application{}, err
	}
	return a, s.Store.AppendAudit(ctx, audit)
}
func (s Service) Accept(ctx context.Context, id, actor string) (Application, error) {
	a, err := s.Store.Application(ctx, id)
	if err != nil {
		return Application{}, err
	}
	o, err := s.Store.Offer(ctx, id)
	if err != nil {
		return Application{}, err
	}
	if a.Status != "offered" || !o.ExpiresAt.After(s.now()) {
		return Application{}, ErrInvalidState
	}
	a.Status = "accepted"
	audit := Audit{ApplicationID: a.ID, Actor: actor, Action: "accepted", Reason: "offer accepted", At: s.now()}
	if store, ok := s.Store.(AcceptanceEventStore); ok {
		return a, store.RecordAcceptanceWithEvent(ctx, a, audit, o)
	}
	if store, ok := s.Store.(AtomicStore); ok {
		return a, store.RecordAcceptance(ctx, a, audit)
	}
	if err = s.Store.SaveApplication(ctx, a); err != nil {
		return Application{}, err
	}
	return a, s.Store.AppendAudit(ctx, audit)
}
