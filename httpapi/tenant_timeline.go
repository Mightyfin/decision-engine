package httpapi

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http"
	"strconv"
)

func (s Server) tenantTimeline(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "decision_workload")
	if !ok {
		return
	}
	a, ok := s.tenantApplication(w, r, p)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit := 20
	for key, values := range q {
		if len(values) != 1 || (key != "limit" && key != "cursor") {
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
		limit = n
	}
	if len(q.Get("cursor")) > 20 {
		write(w, 400, map[string]string{"error": "invalid_cursor"})
		return
	}
	store, ok := s.Applications.(interface {
		TenantTimeline(context.Context, creditrisk.ApplicationListScope, string, string, int) ([]creditrisk.TimelineEvent, error)
	})
	if !ok {
		write(w, 503, map[string]string{"error": "history_unavailable"})
		return
	}
	items, err := store.TenantTimeline(r.Context(), creditrisk.ApplicationListScope{TenantID: p.TenantID, Environment: p.Environment, CallerApplicationID: p.ApplicationID}, a.ID, q.Get("cursor"), limit+1)
	if errors.Is(err, creditrisk.ErrInvalidCursor) {
		write(w, 400, map[string]string{"error": "invalid_cursor"})
		return
	}
	if err != nil {
		write(w, 503, map[string]string{"error": "history_unavailable"})
		return
	}
	more, next := len(items) > limit, ""
	if more {
		items = items[:limit]
		next = items[len(items)-1].ID
	}
	if items == nil {
		items = []creditrisk.TimelineEvent{}
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, map[string]any{"data": items, "page": map[string]any{"has_more": more, "next_cursor": next}})
}
