package httpapi

import (
	"context"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"strings"
	"testing"
)

type documentAccessFixture struct {
	testStore
	app     creditrisk.Application
	bound   bool
	lookups int
}

func (f *documentAccessFixture) EvidenceApplication(context.Context, string) (creditrisk.Application, error) {
	return f.app, nil
}
func (f *documentAccessFixture) HasEvidence(_ context.Context, id, tenant, env, doc, hash string) (bool, error) {
	f.lookups++
	return f.bound && id == "a" && tenant == "t" && env == "sandbox" && doc == "d" && hash == strings.Repeat("a", 64), nil
}
func TestDocumentCaseAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, role, tenant, env, intent, status string
		bound                                   bool
		want                                    int
	}{
		{"staff bound", "credit_analyst", "", "sandbox", "download", "pending_review", true, 200},
		{"staff unbound", "credit_analyst", "", "sandbox", "download", "pending_review", false, 404},
		{"tenant download", "decision_workload", "t", "sandbox", "download", "pending_review", true, 200},
		{"foreign tenant", "decision_workload", "other", "sandbox", "download", "pending_review", true, 403},
		{"foreign environment", "credit_analyst", "", "production", "download", "pending_review", true, 404},
		{"staff cannot upload", "credit_analyst", "", "sandbox", "upload", "pending_review", true, 403},
		{"writer", "credit_evidence_writer", "t", "sandbox", "upload", "awaiting_information", true, 200},
		{"read cannot upload", "decision_workload", "t", "sandbox", "upload", "pending_review", true, 403},
		{"closed case", "credit_evidence_writer", "t", "sandbox", "upload", "declined", true, 409},
		{"unknown intent", "credit_analyst", "", "sandbox", "anything", "pending_review", true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &documentAccessFixture{app: creditrisk.Application{ID: "a", TenantID: "t", Environment: "sandbox", PartyID: "p", RelationshipID: "r", ApplicantRole: "network_participant", Status: tc.status}, bound: tc.bound}
			s := Server{Applications: f, Auth: testAuth{Principal{Subject: "actor", TenantID: tc.tenant, Environment: tc.env, Roles: map[string]bool{tc.role: true}}}}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/credit/applications/a/document-access", strings.NewReader(`{"tenant_id":"t","intent":"`+tc.intent+`","document_id":"d","sha256":"`+strings.Repeat("a", 64)+`"}`)))
			if w.Code != tc.want {
				t.Fatalf("got %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
