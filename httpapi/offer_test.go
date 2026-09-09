package httpapi

import (
	"context"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"strings"
	"testing"
)

type offerReadStore struct{ testStore }

func (s *offerReadStore) EvidenceApplication(ctx context.Context, id string) (creditrisk.Application, error) {
	return s.Application(ctx, id)
}
func TestOfferReadBoundary(t *testing.T) {
	for _, tc := range []struct {
		tenant, environment, status string
		want                        int
	}{
		{"t", "sandbox", "offered", 200}, {"t", "sandbox", "accepted", 200},
		{"other", "sandbox", "offered", 404}, {"t", "production", "offered", 404},
		{"t", "sandbox", "pending_review", 404}, {"t", "sandbox", "declined", 404},
	} {
		t.Run(tc.tenant+tc.environment+tc.status, func(t *testing.T) {
			s := &offerReadStore{testStore{applications: map[string]creditrisk.Application{"a": {ID: "a", TenantID: tc.tenant, Environment: tc.environment, Status: tc.status}}, offers: map[string]creditrisk.Offer{"a": {ApplicationID: "a", Principal: 700000, Total: 735000}}}}
			p := Principal{TenantID: "t", Environment: "sandbox", Roles: map[string]bool{"decision_workload": true}}
			w := httptest.NewRecorder()
			Server{Auth: testAuth{p}, Applications: s}.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/credit/applications/a/offer", nil))
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
			if tc.want == 200 && !strings.Contains(w.Body.String(), `"principal_minor":700000`) {
				t.Fatal(w.Body.String())
			}
			if len(s.audits) != 0 {
				t.Fatal("read mutated audit")
			}
		})
	}
}

func TestReadOnlyTokenCannotAcceptOffer(t *testing.T) {
	p := Principal{TenantID: "t", Environment: "sandbox", Roles: map[string]bool{"decision_workload": true}}
	w := httptest.NewRecorder()
	Server{Auth: testAuth{p}}.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/credit/applications/a/accept", nil))
	if w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
}
