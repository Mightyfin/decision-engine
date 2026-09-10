package httpapi

import (
	"context"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"strings"
	"testing"
)

type caseStore struct {
	testStore
	historyReads int
}

func (s *caseStore) ReviewHistory(_ context.Context, tenant, id string) ([]creditrisk.Audit, error) {
	s.historyReads++
	return []creditrisk.Audit{{Actor: "reviewer", Action: "declined", Reason: "Recorded rationale"}}, nil
}
func TestReviewCaseScopeAndHistory(t *testing.T) {
	for _, tc := range []struct {
		role, tenant string
		status       int
	}{{"credit_analyst", "t", 200}, {"credit_analyst", "other", 404}, {"tenant_admin", "t", 403}} {
		s := &caseStore{testStore: testStore{applications: map[string]creditrisk.Application{"a": {ID: "a", TenantID: "t", Amount: 500000}}}}
		h := Server{Auth: testAuth{Principal{Subject: "analyst", Environment: "sandbox", Roles: map[string]bool{tc.role: true}}}, Applications: s}.Handler()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/internal/tenants/"+tc.tenant+"/credit/applications/a/review", nil))
		if w.Code != tc.status {
			t.Fatal(w.Code, w.Body.String())
		}
		if tc.status == 200 && (!strings.Contains(w.Body.String(), `"actor":"reviewer"`) || !strings.Contains(w.Body.String(), `"offer":null`) || !strings.Contains(w.Body.String(), `"scenario":"unclassified"`) || !strings.Contains(w.Body.String(), `"subject_status":"incomplete"`)) {
			t.Fatal(w.Body.String())
		}
		if tc.status != 200 && s.historyReads != 0 {
			t.Fatal("unauthorised history read")
		}
	}
}
