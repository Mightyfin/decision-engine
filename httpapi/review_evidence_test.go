package httpapi

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"strings"
	"testing"
)

type reviewEvidenceStore struct {
	caseStore
	environment string
	evidence    []creditrisk.Evidence
	readError   error
}

func (s *reviewEvidenceStore) EvidenceApplication(context.Context, string) (creditrisk.Application, error) {
	return creditrisk.Application{ID: "a", TenantID: "t", Environment: s.environment}, nil
}
func (s *reviewEvidenceStore) EvidencePage(_ context.Context, id, tenant, environment, _, _ string, limit int) ([]creditrisk.Evidence, error) {
	if id != "a" || tenant != "t" || environment != "sandbox" || limit != 1 {
		return nil, errors.New("wrong evidence scope")
	}
	return s.evidence, s.readError
}

func TestReviewEvidenceStatusUsesStoredBindings(t *testing.T) {
	for _, tc := range []struct {
		name, environment, want string
		rows                    []creditrisk.Evidence
		err                     error
		status                  int
	}{
		{name: "linked", environment: "sandbox", want: `"document_evidence_status":"linked"`, rows: []creditrisk.Evidence{{DocumentID: "invoice"}}, status: 200},
		{name: "empty", environment: "sandbox", want: `"document_evidence_status":"not_linked"`, status: 200},
		{name: "unavailable", environment: "sandbox", want: "evidence_unavailable", err: errors.New("database down"), status: 503},
		{name: "other environment", environment: "production", want: "not_found", status: 404},
		{name: "legacy", want: "application_environment_unknown", status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &reviewEvidenceStore{caseStore: caseStore{testStore: testStore{applications: map[string]creditrisk.Application{"a": {ID: "a", TenantID: "t"}}}}, environment: tc.environment, evidence: tc.rows, readError: tc.err}
			server := Server{Auth: testAuth{Principal{Environment: "sandbox", Roles: map[string]bool{"credit_analyst": true}}}, Applications: store}
			w := httptest.NewRecorder()
			server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/internal/tenants/t/credit/applications/a/review", nil))
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.want) {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
