package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"strings"
	"testing"
)

type questionnaireStore struct {
	caseStore
	reads   int
	failure error
	legacy  bool
}

func (s *questionnaireStore) ReviewQuestionnaire(_ context.Context, id, tenant, environment string) (*creditrisk.Questionnaire, error) {
	s.reads++
	if id != "a" || tenant != "t" || environment != "sandbox" {
		return nil, creditrisk.ErrNotFound
	}
	if s.failure != nil {
		return nil, s.failure
	}
	if s.legacy {
		return nil, nil
	}
	return &creditrisk.Questionnaire{ProductVersion: 2, Revision: 3, Answers: map[string]json.RawMessage{"income": json.RawMessage(`1234`)}}, nil
}
func TestReviewQuestionnaireBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, role, tenant, env string
		failure                 error
		legacy                  bool
		status, reads           int
	}{
		{"analyst", "credit_analyst", "t", "sandbox", nil, false, 200, 1},
		{"legacy", "credit_analyst", "t", "sandbox", nil, true, 200, 1},
		{"foreign tenant", "credit_analyst", "other", "sandbox", nil, false, 404, 0},
		{"foreign environment", "credit_analyst", "t", "production", nil, false, 404, 1},
		{"tenant role", "tenant_admin", "t", "sandbox", nil, false, 403, 0},
		{"failed read", "credit_analyst", "t", "sandbox", errors.New("private database details"), false, 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &questionnaireStore{caseStore: caseStore{testStore: testStore{applications: map[string]creditrisk.Application{"a": {ID: "a", TenantID: "t", Amount: 1000}}}}, failure: tc.failure, legacy: tc.legacy}
			w := httptest.NewRecorder()
			Server{Auth: testAuth{Principal{Subject: "analyst", Environment: tc.env, Roles: map[string]bool{tc.role: true}}}, Applications: s}.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/internal/tenants/"+tc.tenant+"/credit/applications/a/review", nil))
			if w.Code != tc.status || s.reads != tc.reads {
				t.Fatalf("status=%d reads=%d %s", w.Code, s.reads, w.Body.String())
			}
			body := w.Body.String()
			if tc.status == 200 && !tc.legacy && !strings.Contains(body, `"income":1234`) {
				t.Fatal("answers missing", body)
			}
			if tc.legacy && !strings.Contains(body, `"questionnaire":null`) {
				t.Fatal("legacy misrepresented", body)
			}
			if strings.Contains(body, "private database details") || tc.status != 200 && strings.Contains(body, `"income"`) {
				t.Fatal("leaked questionnaire", body)
			}
		})
	}
}
