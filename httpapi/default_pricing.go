package httpapi

import (
	"context"
	"encoding/json"
	"github.com/Mightyfin/decision-engine/pricing"
	"net/http"
)

func (s Server) bindDefaultPricing(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "credit_policy_admin")
	if !ok {
		return
	}
	if p.ApplicationID != "" || p.Subject == "" {
		write(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	store, ok := s.Policies.(interface {
		BindDefaultPricing(context.Context, string, string, string, pricing.DefaultBinding) error
	})
	if !ok {
		write(w, 503, map[string]string{"error": "pricing_configuration_unavailable"})
		return
	}
	var b pricing.DefaultBinding
	if json.NewDecoder(r.Body).Decode(&b) != nil {
		write(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if err := store.BindDefaultPricing(r.Context(), r.PathValue("tenant_id"), r.PathValue("product_id"), p.Subject, b); err != nil {
		write(w, 422, map[string]string{"error": "binding_rejected"})
		return
	}
	write(w, 200, b)
}
