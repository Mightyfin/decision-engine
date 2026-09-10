package httpapi

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type purchaseFixture struct {
	testStore
	calls int
	actor string
}

func (f *purchaseFixture) Verify(_ context.Context, tenant string, p creditrisk.PurchaseRestriction) (creditrisk.DestinationVerification, error) {
	return creditrisk.DestinationVerification{WalletID: p.DestinationWalletID, PartyID: p.SupplierPartyID, Currency: p.Currency, TenantID: tenant, LegalEntityID: "lender", Environment: "sandbox", Verification: "active_owned_wallet", VerifiedAt: time.Now().UTC()}, nil
}

func (f *purchaseFixture) PurchaseRestriction(context.Context, string, string, string) (creditrisk.PurchaseRestriction, error) {
	f.calls++
	return creditrisk.PurchaseRestriction{}, nil
}
func (f *purchaseFixture) RecordPurchaseRestriction(_ context.Context, _ creditrisk.PurchaseRestriction, _, _, actor, _, _ string) error {
	f.calls++
	f.actor = actor
	return nil
}

const purchaseBody = `{"order_reference":"ORDER-1","supplier_party_id":"supplier","destination_wallet_id":"wallet","document_id":"invoice","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","currency":"ZMW","maximum_amount_minor":1000,"review_revision":"1","reason":"invoice reviewed"}`

func TestPurchaseRestrictionStaffBoundary(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		for _, tc := range []struct {
			tenant, subject, env string
			role                 bool
			status               int
		}{{"", "analyst", "sandbox", true, 200}, {"tenant", "analyst", "sandbox", true, 403}, {"", "analyst", "sandbox", false, 403}, {"", "", "sandbox", true, 403}, {"", "analyst", "", true, 403}} {
			f := &purchaseFixture{}
			server := Server{DestinationVerifier: f, Auth: testAuth{Principal{TenantID: tc.tenant, Subject: tc.subject, Environment: tc.env, Roles: map[string]bool{"credit_analyst": tc.role}}}, Applications: f}
			w := httptest.NewRecorder()
			server.Handler().ServeHTTP(w, httptest.NewRequest(method, "/v1/internal/tenants/t/credit/applications/a/purchase-restriction", strings.NewReader(purchaseBody)))
			if w.Code != tc.status || (tc.status == 403 && f.calls != 0) {
				t.Fatal(method, tc, w.Code, f.calls, w.Body.String())
			}
			if tc.status == 200 && method == "POST" && f.actor != "analyst" {
				t.Fatal("actor not from token")
			}
		}
	}
}
func TestPurchaseRestrictionRejectsOverrides(t *testing.T) {
	for _, body := range []string{strings.Replace(purchaseBody, `"reason":`, `"recorded_by":"forged","reason":`, 1), purchaseBody + `{}`, strings.Replace(purchaseBody, `1000`, `0`, 1)} {
		f := &purchaseFixture{}
		server := Server{Auth: testAuth{Principal{Subject: "analyst", Environment: "sandbox", Roles: map[string]bool{"credit_analyst": true}}}, Applications: f}
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/internal/tenants/t/credit/applications/a/purchase-restriction", strings.NewReader(body)))
		if w.Code != 400 || f.calls != 0 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

func TestTenantCannotReadStaffReviewWithAnalystRole(t *testing.T) {
	f := &purchaseFixture{}
	server := Server{Auth: testAuth{Principal{TenantID: "t", Subject: "tenant-user", Environment: "sandbox", Roles: map[string]bool{"credit_analyst": true}}}, Applications: f}
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/internal/tenants/t/credit/applications/a/review", nil))
	if w.Code != 403 || f.calls != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
}

type destinationVerifierFunc func(context.Context, string, creditrisk.PurchaseRestriction) (creditrisk.DestinationVerification, error)

func (f destinationVerifierFunc) Verify(ctx context.Context, tenant string, p creditrisk.PurchaseRestriction) (creditrisk.DestinationVerification, error) {
	return f(ctx, tenant, p)
}

func TestPurchaseRestrictionVerificationFailureNeverWrites(t *testing.T) {
	for _, failure := range []string{"not_configured", "unavailable", "wrong_tenant", "wrong_environment", "wrong_wallet", "stale"} {
		t.Run(failure, func(t *testing.T) {
			store := &purchaseFixture{}
			server := Server{Auth: testAuth{Principal{Subject: "analyst", Environment: "sandbox", Roles: map[string]bool{"credit_analyst": true}}}, Applications: store}
			if failure != "not_configured" {
				server.DestinationVerifier = destinationVerifierFunc(func(ctx context.Context, tenant string, p creditrisk.PurchaseRestriction) (creditrisk.DestinationVerification, error) {
					proof, _ := store.Verify(ctx, tenant, p)
					switch failure {
					case "unavailable":
						return proof, errors.New("private provider detail")
					case "wrong_tenant":
						proof.TenantID = "another-tenant"
					case "wrong_environment":
						proof.Environment = "production"
					case "wrong_wallet":
						proof.WalletID = "substituted-wallet"
					case "stale":
						proof.VerifiedAt = time.Now().Add(-2 * time.Minute)
					}
					return proof, nil
				})
			}
			w := httptest.NewRecorder()
			server.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/internal/tenants/t/credit/applications/a/purchase-restriction", strings.NewReader(purchaseBody)))
			if w.Code != 503 || store.calls != 0 || strings.Contains(w.Body.String(), "private provider detail") {
				t.Fatal("verification failure must not write or leak provider details", w.Code, store.calls, w.Body.String())
			}
		})
	}
}
