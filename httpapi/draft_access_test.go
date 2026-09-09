package httpapi

import (
	"context"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"strings"
	"testing"
)

type guardedDraftStore struct {
	testStore
	calls int
}

func (s *guardedDraftStore) CheckDraftCaller(_ context.Context, id, tenant, environment, caller string) error {
	s.calls++
	if id != "a" || tenant != "t" || environment != "sandbox" || caller != "app_a" {
		return creditrisk.ErrNotFound
	}
	return nil
}
func TestDraftWorkloadIsolationAlsoProtectsApplicationRead(t *testing.T) {
	for _, app := range []string{"app_a", "app_b"} {
		s := &guardedDraftStore{testStore: testStore{applications: map[string]creditrisk.Application{"a": {ID: "a", TenantID: "t"}}}}
		w := httptest.NewRecorder()
		Server{Auth: testAuth{Principal{TenantID: "t", Environment: "sandbox", ApplicationID: app, Roles: map[string]bool{"decision_workload": true}}}, Applications: s}.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/credit/applications/a", nil))
		want := 200
		if app == "app_b" {
			want = 404
		}
		if w.Code != want || s.calls != 1 {
			t.Fatal(w.Code, s.calls, w.Body.String())
		}
	}
}
func TestReadOnlyWorkloadCannotMutateDraft(t *testing.T) {
	for _, methodPath := range [][2]string{{"POST", "/v1/credit/application-drafts"}, {"PUT", "/v1/credit/applications/a/draft"}, {"POST", "/v1/credit/applications/a/submit"}} {
		p := Principal{Subject: "client", TenantID: "t", Environment: "sandbox", ApplicationID: "app_a", Roles: map[string]bool{"decision_workload": true}}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(methodPath[0], methodPath[1], strings.NewReader(`{}`))
		r.Header.Set("Idempotency-Key", "key")
		Server{Auth: testAuth{p}}.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(methodPath, w.Code, w.Body.String())
		}
	}
}
