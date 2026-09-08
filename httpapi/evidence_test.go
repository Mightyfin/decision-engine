package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEvidenceRouteRequiresDedicatedScopedAccess(t *testing.T) {
	for _, tc := range []struct {
		role, tenant, env string
		status            int
	}{
		{"decision_workload", "t", "sandbox", 403},
		{"credit_analyst", "t", "sandbox", 403},
		{"credit_evidence_writer", "", "sandbox", 403},
		{"credit_evidence_writer", "t", "", 403},
		{"credit_evidence_writer", "t", "sandbox", 503},
	} {
		h := Server{Auth: testAuth{Principal{Subject: "actor", TenantID: tc.tenant, Environment: tc.env, Roles: map[string]bool{tc.role: true}}}}.Handler()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/credit/applications/a/evidence", strings.NewReader(`{"document_id":"doc","sha256":"invalid"}`)))
		if w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
}
