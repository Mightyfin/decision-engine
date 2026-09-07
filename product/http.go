package product

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type tokenContextKey struct{}

func WithBearerToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenContextKey{}, strings.TrimSpace(token))
}

type HTTPStore struct {
	BaseURL string
	Client  *http.Client
}

func (s HTTPStore) Policy(ctx context.Context, tenantID, id string) (Policy, error) {
	token, _ := ctx.Value(tokenContextKey{}).(string)
	if token == "" {
		return Policy{}, fmt.Errorf("product access token unavailable")
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.BaseURL, "/")+"/v1/products/"+id, nil)
	if err != nil {
		return Policy{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := client.Do(req)
	if err != nil {
		return Policy{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return Policy{}, ErrNotFound
	}
	if res.StatusCode != 200 {
		return Policy{}, fmt.Errorf("product engine returned %d", res.StatusCode)
	}
	var out struct {
		ID            string `json:"id"`
		TenantID      string `json:"tenant_id"`
		Code          string `json:"code"`
		Family        string `json:"family"`
		Lifecycle     string `json:"lifecycle"`
		ActiveVersion *int   `json:"active_version"`
		Version       *struct {
			Version       int    `json:"version"`
			Currency      string `json:"currency"`
			Configuration struct {
				MinimumAmount         int64    `json:"minimum_amount_minor"`
				MaximumAmount         int64    `json:"maximum_amount_minor"`
				MinimumTerm           int      `json:"minimum_term_days"`
				MaximumTerm           int      `json:"maximum_term_days"`
				RepaymentIntervalDays int      `json:"repayment_interval_days"`
				GraceDays             int      `json:"grace_days"`
				AllocationOrder       []string `json:"allocation_order"`
				AllowedApplicantRoles []string `json:"allowed_applicant_roles"`
			} `json:"configuration"`
		} `json:"version"`
	}
	if err = json.NewDecoder(res.Body).Decode(&out); err != nil {
		return Policy{}, err
	}
	if out.TenantID != tenantID || out.Lifecycle != "active" || out.Version == nil || out.ActiveVersion == nil || *out.ActiveVersion != out.Version.Version {
		return Policy{}, ErrNotFound
	}
	c := out.Version.Configuration
	return Policy{ID: out.ID, TenantID: out.TenantID, Code: out.Code, Currency: out.Version.Currency, Version: out.Version.Version, MinimumAmount: c.MinimumAmount, MaximumAmount: c.MaximumAmount, MinimumTermDays: c.MinimumTerm, MaximumTermDays: c.MaximumTerm, RepaymentIntervalDays: c.RepaymentIntervalDays, GraceDays: c.GraceDays, AllocationOrder: c.AllocationOrder, AllowedApplicantRoles: c.AllowedApplicantRoles, Active: true}, nil
}
