package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
)

type acceptanceStore struct {
	offerReadStore
	replay  bool
	failure error
	calls   int
	request creditrisk.AcceptanceRequest
}

func (s *acceptanceStore) AcceptOffer(_ context.Context, a creditrisk.Application, _ creditrisk.Offer, _ creditrisk.Audit, in creditrisk.AcceptanceRequest) (creditrisk.Application, bool, error) {
	s.calls++
	s.request = in
	a.Status = "accepted"
	return a, s.replay, s.failure
}

func TestAcceptanceHTTPValidationAndReplay(t *testing.T) {
	for _, tc := range []struct {
		name, body, key string
		failure         error
		replay          bool
		want            int
	}{
		{"valid", `{"quote_id":"q","consent_reference":"consent-1"}`, "acceptance-key-1234", nil, false, 200},
		{"replay", `{"quote_id":"q","consent_reference":"consent-1"}`, "acceptance-key-1234", nil, true, 200},
		{"empty", "", "acceptance-key-1234", nil, false, 400},
		{"consent missing", `{"quote_id":"q"}`, "acceptance-key-1234", nil, false, 400},
		{"unknown", `{"quote_id":"q","consent_reference":"c","approved":true}`, "acceptance-key-1234", nil, false, 400},
		{"missing key", `{"quote_id":"q","consent_reference":"c"}`, "", nil, false, 400},
		{"unavailable", `{"quote_id":"q","consent_reference":"c"}`, "acceptance-key-1234", errors.New("database unavailable"), false, 503},
		{"conflict", `{"quote_id":"q","consent_reference":"c"}`, "acceptance-key-1234", creditrisk.ErrAcceptanceConflict, false, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &acceptanceStore{offerReadStore: offerReadStore{testStore{applications: map[string]creditrisk.Application{"a": {ID: "a", TenantID: "t", Environment: "sandbox", Status: "offered"}}, offers: map[string]creditrisk.Offer{"a": {ApplicationID: "a", QuoteID: "q"}}}}, failure: tc.failure, replay: tc.replay}
			p := Principal{Subject: "caller", TenantID: "t", Environment: "sandbox", ApplicationID: "client", Roles: map[string]bool{"decision_workload": true, "credit_application_writer": true}}
			h := Server{Auth: testAuth{p}, Applications: s, Credit: creditrisk.Service{Store: s}}.Handler()
			r := httptest.NewRequest("POST", "/v1/credit/applications/a/accept", strings.NewReader(tc.body))
			r.Header.Set("Idempotency-Key", tc.key)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
			if tc.want == 400 && s.calls != 0 {
				t.Fatal("invalid input mutated state")
			}
			if tc.replay && w.Header().Get("Idempotent-Replayed") != "true" {
				t.Fatal("missing replay header")
			}
			if s.calls > 0 && (s.request.TenantID != "t" || s.request.Environment != "sandbox" || s.request.CallerApplicationID != "client") {
				t.Fatal("scope not derived from authentication")
			}
			if strings.Contains(w.Body.String(), "database unavailable") {
				t.Fatal("internal error exposed")
			}
		})
	}
}
