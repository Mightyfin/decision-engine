package httpapi

import (
	"context"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"strings"
	"testing"
)

type timelineStore struct {
	offerReadStore
	calls int
}

func (s *timelineStore) TenantTimeline(_ context.Context, scope creditrisk.ApplicationListScope, id, cursor string, limit int) ([]creditrisk.TimelineEvent, error) {
	s.calls++
	if scope.TenantID != "t" || scope.Environment != "sandbox" || scope.CallerApplicationID != "app" || id != "a" || limit != 2 {
		panic("incorrect scope or pagination")
	}
	return []creditrisk.TimelineEvent{{ID: "1", Action: "submitted"}, {ID: "2", Action: "offer"}}, nil
}
func TestTenantTimelineBoundary(t *testing.T) {
	for _, tc := range []struct {
		query, env string
		want       int
	}{{"?limit=1", "sandbox", 200}, {"?limit=0", "sandbox", 400}, {"?limit=1&limit=2", "sandbox", 400}, {"?tenant_id=other", "sandbox", 400}, {"?limit=1", "production", 404}} {
		s := &timelineStore{offerReadStore: offerReadStore{testStore{applications: map[string]creditrisk.Application{"a": {ID: "a", TenantID: "t", Environment: tc.env}}}}}
		p := Principal{TenantID: "t", Environment: "sandbox", ApplicationID: "app", Roles: map[string]bool{"decision_workload": true}}
		w := httptest.NewRecorder()
		Server{Auth: testAuth{p}, Applications: s}.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/credit/applications/a/history"+tc.query, nil))
		if w.Code != tc.want {
			t.Fatal(tc, w.Code, w.Body.String())
		}
		if tc.want == 200 && (!strings.Contains(w.Body.String(), `"next_cursor":"1"`) || strings.Contains(w.Body.String(), `"id":"2"`)) {
			t.Fatal(w.Body.String())
		}
		if tc.want != 200 && s.calls != 0 {
			t.Fatal("unauthorized query")
		}
	}
}
