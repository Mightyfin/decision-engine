package httpapi

import (
	"context"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"testing"
)

type evidenceReadFixture struct {
	testStore
	a     creditrisk.Application
	reads int
}

func (f *evidenceReadFixture) EvidenceApplication(context.Context, string) (creditrisk.Application, error) {
	return f.a, nil
}
func (f *evidenceReadFixture) EvidencePage(context.Context, string, string, string, string, string, int) ([]creditrisk.Evidence, error) {
	f.reads++
	return []creditrisk.Evidence{}, nil
}
func TestEvidenceListIsolation(t *testing.T) {
	for _, tc := range []struct {
		tenant, environment, role string
		status                    int
	}{
		{"t", "sandbox", "credit_analyst", 200}, {"other", "sandbox", "credit_analyst", 404}, {"t", "production", "credit_analyst", 404}, {"t", "", "credit_analyst", 409}, {"t", "sandbox", "decision_workload", 403},
	} {
		f := &evidenceReadFixture{a: creditrisk.Application{ID: "a", TenantID: tc.tenant, Environment: tc.environment}}
		h := Server{Auth: testAuth{Principal{Environment: "sandbox", Roles: map[string]bool{tc.role: true}}}, Applications: f}.Handler()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/internal/tenants/t/credit/applications/a/evidence", nil))
		if w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
		if tc.status != 200 && f.reads != 0 {
			t.Fatal("unauthorised evidence lookup")
		}
	}
}
