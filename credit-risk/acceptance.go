package creditrisk

import (
	"context"
	"errors"
)

var ErrAcceptanceConflict = errors.New("acceptance idempotency key reused with different input")
var ErrAcceptanceUnavailable = errors.New("durable offer acceptance unavailable")

// ConsentReference is the tenant's auditable record of applicant authority,
// not a substitute for MightyFin's manual credit or funding approvals.
type AcceptanceRequest struct {
	TenantID, Environment, CallerApplicationID, Key string
	QuoteID                                         string `json:"quote_id"`
	ConsentReference                                string `json:"consent_reference"`
}

type DurableAcceptanceStore interface {
	AcceptOffer(context.Context, Application, Offer, Audit, AcceptanceRequest) (Application, bool, error)
}

func (s Service) AcceptIdempotent(ctx context.Context, a Application, actor string, request AcceptanceRequest) (Application, bool, error) {
	store, ok := s.Store.(DurableAcceptanceStore)
	if !ok {
		return Application{}, false, ErrAcceptanceUnavailable
	}
	if actor == "" || a.TenantID != request.TenantID || a.Environment != request.Environment || request.CallerApplicationID == "" || request.Key == "" || request.QuoteID == "" || request.ConsentReference == "" {
		return Application{}, false, ErrInvalidState
	}
	offer, err := s.Store.Offer(ctx, a.ID)
	if err != nil {
		return Application{}, false, err
	}
	// The database checks current status, quote and expiry inside the same
	// transaction as replay state, audit and outbox; no read/write race here.
	audit := Audit{ApplicationID: a.ID, Actor: actor, Action: "accepted", Reason: "offer accepted with tenant consent reference", At: s.now()}
	return store.AcceptOffer(ctx, a, offer, audit, request)
}
