package product

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHTTPStoreRejectsNonCreditFamiliesAndInvalidFraming(t *testing.T) {
	valid := `{"id":"prd_1","tenant_id":"ten_1","family":"term_loan","lifecycle":"active","active_version":1,"version":{"version":1,"currency":"ZMW","configuration":{}}}`
	for _, raw := range []string{
		strings.Replace(valid, "term_loan", "wallet_transfer", 1),
		strings.Replace(valid, "term_loan", "wallet_cash_in", 1),
		strings.Replace(valid, "term_loan", "wallet_cash_out", 1),
		strings.Replace(valid, "term_loan", "", 1),
		valid + ` {"extra":true}`,
		valid + strings.Repeat(" ", 2<<20),
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(raw)) }))
		_, err := (HTTPStore{BaseURL: server.URL}).Policy(WithBearerToken(context.Background(), "token"), "ten_1", "prd_1")
		server.Close()
		if err == nil {
			t.Fatal("invalid product response accepted")
		}
	}
}

func TestHTTPStoreRejectsRedirectWithoutSendingToken(t *testing.T) {
	called := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(200) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer source.Close()
	_, err := (HTTPStore{BaseURL: source.URL}).Policy(WithBearerToken(context.Background(), "test-token"), "t", "p")
	if !errors.Is(err, ErrUnavailable) || called {
		t.Fatal("credential redirect followed", called, err)
	}
}

func TestHTTPStoreSeparatesFailureFromValidation(t *testing.T) {
	for _, status := range []int{401, 403, 404, 500, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		_, err := (HTTPStore{BaseURL: server.URL}).Policy(WithBearerToken(context.Background(), "token"), "t", "p")
		server.Close()
		want := ErrUnavailable
		if status == 401 || status == 403 {
			want = ErrAccessDenied
		}
		if status == 404 {
			want = ErrNotFound
		}
		if !errors.Is(err, want) {
			t.Fatal(status, err)
		}
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
