package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// Exercises signature verification and discovery/JWKS, not a mocked Principal.
// The issuer and keys are isolated test fixtures, not production credentials.
func TestSignedWorkloadPermissions(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL
	verifier, err := NewOIDCVerifier(context.Background(), issuer, "decision-test", "sandbox")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "test"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, scope, app, env, audience string
		expired, invalid, writer        bool
	}{
		{"read only", "credit:read", "app_a", "sandbox", "decision-test", false, false, false},
		{"write", "credit:write", "app_a", "sandbox", "decision-test", false, false, true},
		{"human cannot draft", "credit:write", "", "sandbox", "decision-test", false, false, false},
		{"wrong environment", "credit:write", "app_a", "production", "decision-test", false, true, false},
		{"wrong audience", "credit:write", "app_a", "sandbox", "other", false, true, false},
		{"expired", "credit:write", "app_a", "sandbox", "decision-test", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expires := time.Now().Add(time.Minute)
			if tc.expired {
				expires = time.Now().Add(-time.Hour)
			}
			payload, _ := json.Marshal(map[string]any{"iss": issuer, "aud": tc.audience, "sub": "fixture", "exp": expires.Unix(), "tenant_id": "tenant_a", "application_id": tc.app, "environment": tc.env, "scope": tc.scope})
			object, err := signer.Sign(payload)
			if err != nil {
				t.Fatal(err)
			}
			token, err := object.CompactSerialize()
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			p, err := verifier.Authenticate(r)
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid token accepted")
				}
				return
			}
			if err != nil || p.ApplicationID != tc.app || p.TenantID != "tenant_a" || p.Roles["credit_application_writer"] != tc.writer {
				t.Fatal("claims/permissions mismatch", p, err)
			}
		})
	}
}
