package httpapi

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http"
)

// Tenant reads and acceptance require persisted environment provenance, even
// for legacy applications that have no draft/caller ownership record.
func (s Server) tenantApplication(w http.ResponseWriter, r *http.Request, p Principal) (creditrisk.Application, bool) {
	if p.TenantID == "" || p.Environment == "" {
		write(w, 403, map[string]string{"error": "forbidden"})
		return creditrisk.Application{}, false
	}
	store, ok := s.Applications.(interface {
		EvidenceApplication(context.Context, string) (creditrisk.Application, error)
	})
	if !ok {
		write(w, 503, map[string]string{"error": "application_unavailable"})
		return creditrisk.Application{}, false
	}
	a, err := store.EvidenceApplication(r.Context(), r.PathValue("id"))
	if errors.Is(err, creditrisk.ErrNotFound) || (err == nil && (a.TenantID != p.TenantID || a.Environment != p.Environment)) {
		write(w, 404, map[string]string{"error": "not_found"})
		return creditrisk.Application{}, false
	}
	if err != nil {
		write(w, 503, map[string]string{"error": "application_unavailable"})
		return creditrisk.Application{}, false
	}
	return a, true
}
