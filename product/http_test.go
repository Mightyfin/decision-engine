package product

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPStoreMapsActiveLoanProduct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("token not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"prd_1","tenant_id":"ten_1","code":"SALARY_ADVANCE","family":"salary_advance","lifecycle":"active","active_version":2,"version":{"version":2,"currency":"ZMW","configuration":{"minimum_amount_minor":10000,"maximum_amount_minor":5000000,"minimum_term_days":30,"maximum_term_days":180,"repayment_interval_days":30,"grace_days":3,"allocation_order":["penalty","fees","interest","principal"]}}}`))
	}))
	defer server.Close()
	p, err := (HTTPStore{BaseURL: server.URL}).Policy(WithBearerToken(context.Background(), "test-token"), "ten_1", "prd_1")
	if err != nil || p.Version != 2 || p.Currency != "ZMW" || p.MinimumAmount != 10000 || p.GraceDays != 3 {
		t.Fatalf("unexpected policy %#v error=%v", p, err)
	}
}

func TestHTTPStoreRejectsCrossTenantResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"prd_1","tenant_id":"ten_other","lifecycle":"active","active_version":1,"version":{"version":1,"currency":"ZMW","configuration":{}}}`))
	}))
	defer server.Close()
	if _, err := (HTTPStore{BaseURL: server.URL}).Policy(WithBearerToken(context.Background(), "test-token"), "ten_1", "prd_1"); err == nil {
		t.Fatal("cross-tenant response accepted")
	}
}
