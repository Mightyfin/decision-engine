package httpapi

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"strings"
	"testing"
)

type commercialHTTPStore struct {
	offerReadStore
	calls   int
	failure error
	replay  bool
}

func (s *commercialHTTPStore) CommercialReview(context.Context, string, creditrisk.SubmissionIdentity) (creditrisk.CommercialReview, error) {
	s.calls++
	return creditrisk.CommercialReview{ApplicationID: "a", Revision: 1, Decision: "pending"}, s.failure
}
func (s *commercialHTTPStore) RecordCommercialReview(_ context.Context, r creditrisk.CommercialReview, a creditrisk.Audit, sc creditrisk.SubmissionIdentity) (creditrisk.CommercialReview, bool, error) {
	s.calls++
	if a.Actor != "caller" || sc.TenantID != "t" || sc.Environment != "sandbox" || sc.CallerApplicationID != "client" || sc.Hash == "" {
		return r, false, errors.New("untrusted provenance")
	}
	return r, s.replay, s.failure
}

func TestCommercialReviewHTTPGates(t *testing.T) {
	for _, tc := range []struct {
		name, method, body, tenant string
		reviewer, replay           bool
		failure                    error
		want                       int
	}{
		{name: "read", method: "GET", want: 200},
		{name: "record", reviewer: true, want: 200},
		{name: "replay", reviewer: true, replay: true, want: 200},
		{name: "ordinary writer cannot approve", want: 403},
		{name: "foreign tenant", reviewer: true, tenant: "other", want: 404},
		{name: "forged staff", reviewer: true, body: `{"approved_by":"staff"}`, want: 400},
		{name: "invalid body", reviewer: true, body: `{}`, want: 400},
		{name: "stale snapshot", reviewer: true, failure: creditrisk.ErrInvalidState, want: 409},
		{name: "outage", reviewer: true, failure: errors.New("private SQL details"), want: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &commercialHTTPStore{offerReadStore: offerReadStore{testStore{applications: map[string]creditrisk.Application{"a": {ID: "a", TenantID: "t", Environment: "sandbox", Status: "draft"}}}}, failure: tc.failure, replay: tc.replay}
			p := Principal{Subject: "caller", TenantID: "t", Environment: "sandbox", ApplicationID: "client", Roles: map[string]bool{"decision_workload": true, "credit_application_writer": true, "credit_commercial_reviewer": tc.reviewer}}
			if tc.tenant != "" {
				p.TenantID = tc.tenant
			}
			method := tc.method
			if method == "" {
				method = "POST"
			}
			body := tc.body
			if body == "" {
				body = `{"revision":1,"snapshot_hash":"` + strings.Repeat("a", 64) + `","decision":"approved","review_reference":"tenant-review","reason":"Verified tenant business review"}`
			}
			r := httptest.NewRequest(method, "/v1/credit/applications/a/commercial-review", strings.NewReader(body))
			r.Header.Set("Idempotency-Key", "commercial-request-1234")
			w := httptest.NewRecorder()
			Server{Auth: testAuth{p}, Applications: s}.Handler().ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
			if (tc.want == 400 || tc.want == 403 || tc.want == 404) && s.calls != 0 {
				t.Fatal("rejected request reached review store")
			}
			if tc.replay && w.Header().Get("Idempotent-Replayed") != "true" {
				t.Fatal("missing replay")
			}
			if strings.Contains(w.Body.String(), "private SQL") {
				t.Fatal("private error exposed")
			}
		})
	}
}
