package httpapi

import (
	"context"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"strings"
	"testing"
)

type pageStore struct {
	testStore
	tenant, after string
	limit         int
}

func (s *pageStore) ReviewQueuePage(_ context.Context, tenant string, limit int, after string) ([]creditrisk.Application, error) {
	s.tenant = tenant
	s.limit = limit
	s.after = after
	return []creditrisk.Application{{ID: "one", Amount: 500000}, {ID: "two", Amount: 100000}}, nil
}
func TestQueuePageContract(t *testing.T) {
	s := &pageStore{}
	h := Server{Auth: testAuth{Principal{Subject: "analyst", Roles: map[string]bool{"credit_analyst": true}}}, Applications: s}.Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/internal/tenants/tenant-a/credit/review-queue/page?limit=1&after=previous", nil))
	if w.Code != 200 || s.tenant != "tenant-a" || s.limit != 2 || s.after != "previous" || !strings.Contains(w.Body.String(), `"next_cursor":"one"`) || !strings.Contains(w.Body.String(), `"amount_minor":500000`) || strings.Contains(w.Body.String(), `"id":"two"`) {
		t.Fatal(w.Code, w.Body.String(), s)
	}
	for _, limit := range []string{"0", "101", "oops"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/internal/tenants/t/credit/review-queue/page?limit="+limit, nil))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	denied := Server{Auth: testAuth{Principal{Roles: map[string]bool{"tenant_admin": true}}}, Applications: s}.Handler()
	w = httptest.NewRecorder()
	denied.ServeHTTP(w, httptest.NewRequest("GET", "/v1/internal/tenants/t/credit/review-queue/page", nil))
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
