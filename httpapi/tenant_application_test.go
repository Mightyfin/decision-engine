package httpapi

import (
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"testing"
)

func TestTenantApplicationProvenance(t *testing.T) {
	for _, env := range []string{"sandbox", "production", ""} {
		s := &offerReadStore{testStore{applications: map[string]creditrisk.Application{"a": {ID: "a", TenantID: "t", Environment: env}}}}
		p := Principal{TenantID: "t", Environment: "sandbox", Roles: map[string]bool{"decision_workload": true, "credit_application_writer": true}}
		h := Server{Auth: testAuth{p}, Applications: s}.Handler()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/credit/applications/a", nil))
		want := 404
		if env == "sandbox" {
			want = 200
		}
		if w.Code != want {
			t.Fatal(env, w.Code, w.Body.String())
		}
		if env != "sandbox" {
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/credit/applications/a/accept", nil))
			if w.Code != 404 {
				t.Fatal("acceptance crossed environment", env, w.Code)
			}
		}
	}
}
