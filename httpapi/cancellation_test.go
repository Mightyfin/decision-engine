package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
)

type cancellationHTTPStore struct {
	offerReadStore
	calls    int
	failure  error
	replay   bool
	identity creditrisk.SubmissionIdentity
	audit    creditrisk.Audit
}

func (s *cancellationHTTPStore) CancelApplication(_ context.Context, a creditrisk.Application, audit creditrisk.Audit, identity creditrisk.SubmissionIdentity) (creditrisk.Application, bool, error) {
	s.calls++
	s.identity, s.audit = identity, audit
	a.Status = "cancelled"
	return a, s.replay, s.failure
}

func TestCancellationHTTPAuthorityValidationAndReplay(t *testing.T) {
	for _, tc := range []struct {
		name, body, key, tenant, environment string
		readOnly, replay                     bool
		failure                              error
		want                                 int
	}{
		{name: "valid", want: 200},
		{name: "replay", replay: true, want: 200},
		{name: "short reason", body: `{"reason":"short"}`, want: 400},
		{name: "unknown field", body: `{"reason":"Applicant changed plans","approved":true}`, want: 400},
		{name: "trailing JSON", body: `{"reason":"Applicant changed plans"}{}`, want: 400},
		{name: "short key", key: "short", want: 400},
		{name: "foreign tenant", tenant: "other", want: 404},
		{name: "foreign environment", environment: "production", want: 404},
		{name: "read only", readOnly: true, want: 403},
		{name: "invalid state", failure: creditrisk.ErrInvalidState, want: 409},
		{name: "key conflict", failure: creditrisk.ErrDraftKeyConflict, want: 409},
		{name: "outage", failure: errors.New("private database stack trace"), want: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &cancellationHTTPStore{offerReadStore: offerReadStore{testStore{applications: map[string]creditrisk.Application{"a": {ID: "a", TenantID: "t", Environment: "sandbox", Status: "pending_review"}}}}, failure: tc.failure, replay: tc.replay}
			p := Principal{Subject: "caller", TenantID: "t", Environment: "sandbox", ApplicationID: "client", Roles: map[string]bool{"decision_workload": true, "credit_application_writer": !tc.readOnly}}
			if tc.tenant != "" {
				p.TenantID = tc.tenant
			}
			if tc.environment != "" {
				p.Environment = tc.environment
			}
			body, key := tc.body, tc.key
			if body == "" {
				body = `{"reason":"Applicant changed plans"}`
			}
			if key == "" {
				key = "cancel-request-123456"
			}
			req := httptest.NewRequest("POST", "/v1/credit/applications/a/cancel", strings.NewReader(body))
			req.Header.Set("Idempotency-Key", key)
			w := httptest.NewRecorder()
			Server{Auth: testAuth{p}, Applications: s}.Handler().ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
			if tc.want == 400 || tc.want == 403 || tc.want == 404 {
				if s.calls != 0 {
					t.Fatal("invalid or unauthorized request reached mutation")
				}
			}
			if s.calls > 0 && (s.identity.TenantID != "t" || s.identity.Environment != "sandbox" || s.identity.CallerApplicationID != "client" || s.audit.Actor != "caller" || s.identity.Hash == "") {
				t.Fatal("missing trusted scope or provenance")
			}
			if tc.replay && w.Header().Get("Idempotent-Replayed") != "true" {
				t.Fatal("missing replay header")
			}
			if strings.Contains(w.Body.String(), "private database") {
				t.Fatal("internal failure leaked")
			}
		})
	}
}
