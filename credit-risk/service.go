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
	ID, TenantID, ProductPolicyID, RelationshipID, PartyID, ApplicantRole, WalletID, Origin, Currency, Purpose, Status string
	Amount                                                                                                             int64
	TermDays                                                                                                           int
	ProductPolicyVersion                                                                                               int
	SubmittedAt                                                                                                        time.Time
}
type Offer struct {
	ApplicationID, QuoteID                     string
	ProductPolicyVersion, PricingPolicyVersion int
	Principal, Interest, Fees, Total           int64
	TermDays                                   int
	InstallmentCount, RepaymentIntervalDays    int
	GraceDays, PenaltyRateBPS, PenaltyCapBPS   int
	PenaltyBasis                               string
	AllocationOrder                            []string
	ExpiresAt                                  time.Time
}
type Exposure struct{ ApprovedLimit, Reserved, Utilised int64 }
type Audit struct {
	ApplicationID, Actor, Action, Reason string
	At                                   time.Time
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
	p, err := s.Products.Validate(ctx, a.TenantID, a.ProductPolicyID, a.Currency, a.Amount, a.TermDays)
	if err != nil {
		return Application{}, err
	}
	if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.RelationshipID) == "" || strings.TrimSpace(a.Purpose) == "" {
		return Application{}, fmt.Errorf("application identity, relationship and purpose are required")
	}
	if a.ApplicantRole != "" && a.ApplicantRole != "network_participant" && a.ApplicantRole != "partner_organisation" {
		return Application{}, fmt.Errorf("unsupported applicant role")
	}
	if a.PartyID != "" && a.ApplicantRole == "" {
		return Application{}, fmt.Errorf("party identity requires an applicant role")
	}
	if a.Origin == "" {
		a.Origin = "legacy"
	}
	if !originPattern.MatchString(a.Origin) {
		return Application{}, fmt.Errorf("unsupported credit origin")
	}
	a.Status = "pending_review"
	a.ProductPolicyVersion = p.Version
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
		productPolicy, e := s.Products.Validate(ctx, a.TenantID, a.ProductPolicyID, a.Currency, a.Amount, a.TermDays)
		if e != nil {
			return Application{}, e
		}
		q, e := s.Pricing(policy, pricing.ScheduleTerms{RepaymentIntervalDays: productPolicy.RepaymentIntervalDays, GraceDays: productPolicy.GraceDays, AllocationOrder: productPolicy.AllocationOrder}, a.Amount, a.TermDays, s.now())
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
