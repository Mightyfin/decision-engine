// Package auth verifies workload and staff tokens at the Decision Engine boundary.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Mightyfin/decision-engine/httpapi"
	"github.com/coreos/go-oidc/v3/oidc"
)

var ErrInvalidToken = errors.New("invalid access token")

type OIDCVerifier struct {
	verifier    *oidc.IDTokenVerifier
	environment string
}

type claims struct {
	Subject     string `json:"sub"`
	TenantID    string `json:"tenant_id"`
	Environment string `json:"environment"`
	Scope       string `json:"scope"`
	RealmAccess struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}

func NewOIDCVerifier(ctx context.Context, issuer, audience, environment string) (*OIDCVerifier, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	return &OIDCVerifier{verifier: provider.Verifier(&oidc.Config{ClientID: audience}), environment: environment}, nil
}

func (v *OIDCVerifier) Authenticate(r *http.Request) (httpapi.Principal, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return httpapi.Principal{}, ErrInvalidToken
	}
	token, err := v.verifier.Verify(r.Context(), strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		return httpapi.Principal{}, ErrInvalidToken
	}
	var c claims
	if token.Claims(&c) != nil || c.Subject == "" || c.TenantID == "" || c.Environment != v.environment {
		return httpapi.Principal{}, ErrInvalidToken
	}
	roles := map[string]bool{}
	for _, role := range c.RealmAccess.Roles {
		roles[role] = true
	}
	for _, scope := range strings.Fields(c.Scope) {
		switch scope {
		case "decision:submit", "decision:read":
			roles["decision_workload"] = true
		case "decision:review":
			roles["credit_analyst"] = true
		}
	}
	return httpapi.Principal{Subject: c.Subject, TenantID: c.TenantID, Roles: roles}, nil
}
