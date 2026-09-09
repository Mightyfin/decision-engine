package httpapi

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http"
	"strconv"
)

func (s Server) listApplications(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "decision_workload")
	if !ok {
		return
	}
	if p.TenantID == "" || p.Environment == "" {
		write(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	q := r.URL.Query()
	f := creditrisk.ApplicationListFilter{Limit: 20, Cursor: q.Get("cursor"), Status: q.Get("status"), RelationshipID: q.Get("relationship_id")}
	for key, values := range q {
		if len(values) != 1 || (key != "limit" && key != "cursor" && key != "status" && key != "relationship_id") {
			write(w, 400, map[string]string{"error": "invalid_filter"})
			return
		}
	}
	if q.Has("limit") {
		n, err := strconv.Atoi(q.Get("limit"))
		if err != nil || n < 1 || n > 100 {
			write(w, 400, map[string]string{"error": "invalid_limit"})
			return
		}
		f.Limit = n
	}
	if len(f.Cursor) > 128 || len(f.RelationshipID) > 128 || len(f.Status) > 64 {
		write(w, 400, map[string]string{"error": "invalid_filter"})
		return
	}
	store, ok := s.Applications.(interface {
		ListApplications(context.Context, creditrisk.ApplicationListScope, creditrisk.ApplicationListFilter) ([]creditrisk.Application, error)
	})
	if !ok {
		write(w, 503, map[string]string{"error": "listing_unavailable"})
		return
	}
	limit := f.Limit
	f.Limit++
	items, err := store.ListApplications(r.Context(), creditrisk.ApplicationListScope{TenantID: p.TenantID, Environment: p.Environment, CallerApplicationID: p.ApplicationID}, f)
	if errors.Is(err, creditrisk.ErrInvalidCursor) {
		write(w, 400, map[string]string{"error": "invalid_cursor"})
		return
	}
	if err != nil {
		write(w, 503, map[string]string{"error": "listing_unavailable"})
		return
	}
	next := ""
	more := len(items) > limit
	if more {
		items = items[:limit]
		next = items[len(items)-1].ID
	}
	if items == nil {
		items = []creditrisk.Application{}
	}
	write(w, 200, map[string]any{"data": items, "page": map[string]any{"has_more": more, "next_cursor": next}})
}
