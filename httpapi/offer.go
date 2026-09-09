package httpapi

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http"
)

// Offers are immutable priced terms, not a funding instruction or wallet balance.
func (s Server) getOffer(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "decision_workload")
	if !ok {
		return
	}
	if p.TenantID == "" || p.Environment == "" {
		write(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	store, ok := s.Applications.(interface {
		EvidenceApplication(context.Context, string) (creditrisk.Application, error)
		Offer(context.Context, string) (creditrisk.Offer, error)
	})
	if !ok {
		write(w, 503, map[string]string{"error": "offer_unavailable"})
		return
	}
	a, e := store.EvidenceApplication(r.Context(), r.PathValue("id"))
	if errors.Is(e, creditrisk.ErrNotFound) {
		write(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if e != nil {
		write(w, 503, map[string]string{"error": "offer_unavailable"})
		return
	}
	if a.TenantID != p.TenantID || a.Environment != p.Environment || (a.Status != "offered" && a.Status != "accepted") {
		write(w, 404, map[string]string{"error": "not_found"})
		return
	}
	offer, e := store.Offer(r.Context(), a.ID)
	if errors.Is(e, creditrisk.ErrNotFound) {
		write(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if e != nil {
		write(w, 503, map[string]string{"error": "offer_unavailable"})
		return
	}
	write(w, 200, offer)
}
