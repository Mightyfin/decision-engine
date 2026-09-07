package pricing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type tokenKey struct{}

func WithBearerToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenKey{}, strings.TrimSpace(token))
}

type QuoteRequest struct {
	PricingPolicyID, ProductPolicyID, Currency                       string
	ProductPolicyVersion, TermDays, RepaymentIntervalDays, GraceDays int
	Amount                                                           int64
	AllocationOrder                                                  []string
}
type HTTPClient struct {
	BaseURL string
	Client  *http.Client
}

func (c HTTPClient) Quote(ctx context.Context, in QuoteRequest) (Quote, error) {
	token, _ := ctx.Value(tokenKey{}).(string)
	if token == "" {
		return Quote{}, fmt.Errorf("pricing access token unavailable")
	}
	body, _ := json.Marshal(map[string]any{"policy_id": in.PricingPolicyID, "product_id": in.ProductPolicyID, "product_version": in.ProductPolicyVersion, "quote_type": "credit_offer", "currency": in.Currency, "amount_minor": in.Amount, "term_days": in.TermDays, "repayment_interval_days": in.RepaymentIntervalDays})
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/v1/pricing/quotes", bytes.NewReader(body))
	if e != nil {
		return Quote{}, e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	res, e := client.Do(req)
	if e != nil {
		return Quote{}, e
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return Quote{}, fmt.Errorf("pricing engine returned %d", res.StatusCode)
	}
	var out struct {
		ID string `json:"id"`; PolicyID string `json:"policy_id"`; ProductID string `json:"product_id"`; Currency string `json:"currency"`; PenaltyBasis string `json:"penalty_basis"`
		PolicyVersion int `json:"policy_version"`; ProductVersion int `json:"product_version"`; InstallmentCount int `json:"installment_count"`; RepaymentIntervalDays int `json:"repayment_interval_days"`
		PrincipalMinor int64 `json:"principal_minor"`; InterestMinor int64 `json:"interest_minor"`; FeeMinor int64 `json:"fee_minor"`; TotalMinor int64 `json:"total_minor"`; PenaltyRateBPS int64 `json:"penalty_rate_bps"`; PenaltyCapBPS int64 `json:"penalty_cap_bps"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if e = json.NewDecoder(res.Body).Decode(&out); e != nil {
		return Quote{}, e
	}
	if out.PolicyID != in.PricingPolicyID || out.ProductID != in.ProductPolicyID || out.ProductVersion != in.ProductPolicyVersion {
		return Quote{}, fmt.Errorf("pricing quote reference mismatch")
	}
	return Quote{ID: out.ID, ProductPolicyID: out.ProductID, Currency: out.Currency, ProductPolicyVersion: out.ProductVersion, PricingPolicyVersion: out.PolicyVersion, Principal: out.PrincipalMinor, Interest: out.InterestMinor, Fees: out.FeeMinor, Total: out.TotalMinor, InstallmentCount: out.InstallmentCount, RepaymentIntervalDays: out.RepaymentIntervalDays, GraceDays: in.GraceDays, PenaltyRateBPS: int(out.PenaltyRateBPS), PenaltyCapBPS: int(out.PenaltyCapBPS), PenaltyBasis: out.PenaltyBasis, AllocationOrder: append([]string(nil), in.AllocationOrder...), ExpiresAt: out.ExpiresAt}, nil
}
