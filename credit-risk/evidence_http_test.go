package creditrisk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocumentResponseMustMatchCase(t *testing.T) {
	for _, name := range []string{"valid", "tenant", "environment", "party", "digest", "scan", "redirect"} {
		t.Run(name, func(t *testing.T) {
			a := Application{ID: "a", TenantID: "tenant", Environment: "sandbox", PartyID: "party"}
			digest := strings.Repeat("a", 64)
			d := map[string]string{"id": "doc", "party_id": "party", "owner_type": "PARTY", "owner_id": "party", "sha256": digest, "status": "available", "scan_status": "clean", "document_type": "BANK_STATEMENT"}
			out := map[string]any{"tenant_id": "tenant", "environment": "sandbox", "verification": "available_clean_version", "document": d}
			switch name {
			case "tenant":
				out["tenant_id"] = "foreign"
			case "environment":
				out["environment"] = "production"
			case "party":
				d["party_id"] = "foreign"
			case "digest":
				d["sha256"] = "bad"
			case "scan":
				d["scan_status"] = "pending"
			}
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if name == "redirect" {
					http.Redirect(w, r, "/other", 302)
					return
				}
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing delegated token")
				}
				json.NewEncoder(w).Encode(out)
			}))
			defer srv.Close()
			_, err := (HTTPDocumentVerifier{BaseURL: srv.URL, Token: "test-token"}).VerifyEvidence(context.Background(), a, "doc", digest)
			if (err == nil) != (name == "valid") {
				t.Fatal(name, err)
			}
			if calls != 1 {
				t.Fatal("redirect followed")
			}
		})
	}
}
