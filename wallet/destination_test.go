package wallet

import (
	"context"
	"encoding/json"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDestinationClientRejectsMismatchedAndStaleProof(t *testing.T) {
	for _, change := range []string{"", "wallet_id", "legal_entity_id", "tenant_id", "party_id", "currency", "environment", "verification", "stale", "future", "trailing", "redirect", "token_failure", "token_empty", "wallet_failure", "oversized"} {
		t.Run(change, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					if change == "token_failure" {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					if change == "token_empty" {
						w.Write([]byte(`{}`))
						return
					}
					r.ParseForm()
					if r.Form.Get("scope") != "wallet.destination.verify" {
						t.Error("incorrect scope")
					}
					json.NewEncoder(w).Encode(map[string]string{"access_token": "token"})
					return
				}
				if change == "token_failure" || change == "token_empty" {
					t.Error("wallet called without a valid token")
				}
				if r.URL.Path != "/v1/internal/wallet-destinations/verify" || r.Header.Get("X-Acting-Tenant-Id") != "tenant" || r.Header.Get("Authorization") != "Bearer token" {
					t.Error("incorrect binding")
				}
				body := map[string]string{}
				json.NewDecoder(r.Body).Decode(&body)
				if body["wallet_id"] != "wallet" || body["party_id"] != "supplier" || body["legal_entity_id"] != "lender" {
					t.Error("incorrect command", body)
				}
				proof := map[string]string{"wallet_id": "wallet", "legal_entity_id": "lender", "tenant_id": "tenant", "party_id": "supplier", "currency": "ZMW", "environment": "sandbox", "verification": "active_owned_wallet", "verified_at": time.Now().UTC().Format(time.RFC3339Nano)}
				switch change {
				case "wallet_failure":
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				case "oversized":
					proof["padding"] = strings.Repeat("x", 65536)
				case "stale":
					proof["verified_at"] = time.Now().Add(-2 * time.Minute).Format(time.RFC3339Nano)
				case "future":
					proof["verified_at"] = time.Now().Add(time.Minute).Format(time.RFC3339Nano)
				case "redirect":
					w.Header().Set("Location", "/unexpected")
					w.WriteHeader(307)
					return
				case "", "trailing":
				default:
					proof[change] = "wrong"
				}
				json.NewEncoder(w).Encode(proof)
				if change == "trailing" {
					w.Write([]byte(`{}`))
				}
			}))
			defer server.Close()
			client := DestinationClient{BaseURL: server.URL, TokenURL: server.URL + "/token", ClientID: "client", ClientSecret: "secret", ApplicationID: "app", LegalEntityID: "lender", Environment: "sandbox"}
			request := creditrisk.PurchaseRestriction{ApplicationID: "application", OrderReference: "order", SupplierPartyID: "supplier", DestinationWalletID: "wallet", DocumentID: "invoice", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Currency: "ZMW", MaximumAmountMinor: 100}
			proof, err := client.Verify(context.Background(), "tenant", request)
			if change == "" {
				if err != nil || proof.WalletID != "wallet" {
					t.Fatal(proof, err)
				}
			} else if err == nil {
				t.Fatal("invalid proof accepted", change)
			}
		})
	}
}
