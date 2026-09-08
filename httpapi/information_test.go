package httpapi

import (
	"context"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"strings"
	"testing"
)

type informationFixture struct {
	testStore
	a        creditrisk.Application
	writes   int
	resubmit bool
}

func (f *informationFixture) EvidenceApplication(context.Context, string) (creditrisk.Application, error) {
	return f.a, nil
}
func (f *informationFixture) ReviewRevision(context.Context, string, string) (string, error) {
	return "7", nil
}
func (f *informationFixture) InformationMessage(context.Context, string, string) (string, error) {
	return "Provide bank statements", nil
}
func (f *informationFixture) ChangeInformationState(_ context.Context, id, tenant, env, actor, reason, revision string, resubmit bool) error {
	f.writes++
	f.resubmit = resubmit
	return nil
}
func TestInformationWorkflowScope(t *testing.T) {
	for _, tc := range []struct {
		method, path, role, tenant, environment string
		want                                    int
	}{
		{"POST", "/v1/internal/tenants/t/credit/applications/a/information-request", "credit_analyst", "", "sandbox", 200},
		{"POST", "/v1/credit/applications/a/resubmit", "credit_evidence_writer", "t", "sandbox", 200},
		{"POST", "/v1/credit/applications/a/resubmit", "decision_workload", "t", "sandbox", 403},
		{"POST", "/v1/credit/applications/a/resubmit", "credit_evidence_writer", "foreign", "sandbox", 404},
		{"POST", "/v1/credit/applications/a/resubmit", "credit_evidence_writer", "t", "production", 404},
		{"GET", "/v1/credit/applications/a/information-request", "decision_workload", "t", "sandbox", 200},
	} {
		f := &informationFixture{a: creditrisk.Application{ID: "a", TenantID: "t", Environment: "sandbox"}}
		s := Server{Applications: f, Auth: testAuth{Principal{Subject: "actor", TenantID: tc.tenant, Environment: tc.environment, Roles: map[string]bool{tc.role: true}}}}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"reason":"Please provide bank statements","review_revision":"7"}`)))
		if w.Code != tc.want {
			t.Fatal(tc, w.Code, w.Body.String())
		}
		if tc.want != 200 && f.writes != 0 {
			t.Fatal("unauthorised mutation")
		}
	}
}
