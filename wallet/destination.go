package wallet

import (
	"context"
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("wallet destination verification unavailable")

type DestinationClient struct {
	BaseURL, TokenURL, ClientID, ClientSecret, ApplicationID, LegalEntityID, Environment string
	HTTP                                                                                 *http.Client
}

func (c DestinationClient) Verify(ctx context.Context, tenant string, p creditrisk.PurchaseRestriction) (creditrisk.DestinationVerification, error) {
	var proof creditrisk.DestinationVerification
	if c.BaseURL == "" || c.TokenURL == "" || c.ClientID == "" || c.ClientSecret == "" || c.ApplicationID == "" || c.LegalEntityID == "" || c.Environment == "" || tenant == "" || p.Validate() != nil {
		return proof, ErrUnavailable
	}
	client := &http.Client{Timeout: 5 * time.Second}
	if c.HTTP != nil {
		copy := *c.HTTP
		client = &copy
	}
	if client.Timeout <= 0 {
		client.Timeout = 5 * time.Second
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	form := url.Values{"grant_type": {"client_credentials"}, "scope": {"wallet.destination.verify"}}
	req, err := http.NewRequestWithContext(ctx, "POST", c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return proof, ErrUnavailable
	}
	req.SetBasicAuth(c.ClientID, c.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if !read(client, req, &token) || token.AccessToken == "" {
		return proof, ErrUnavailable
	}
	body, _ := json.Marshal(map[string]string{"legal_entity_id": c.LegalEntityID, "wallet_id": p.DestinationWalletID, "party_id": p.SupplierPartyID, "currency": p.Currency})
	req, err = http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.BaseURL, "/")+"/v1/internal/wallet-destinations/verify", strings.NewReader(string(body)))
	if err != nil {
		return proof, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Acting-Tenant-Id", tenant)
	req.Header.Set("X-Acting-Application-Id", c.ApplicationID)
	if !read(client, req, &proof) {
		return proof, ErrUnavailable
	}
	p.DestinationVerification = &proof
	if proof.LegalEntityID != c.LegalEntityID || !p.HasCurrentDestination(tenant, c.Environment, time.Now().UTC()) {
		return creditrisk.DestinationVerification{}, ErrUnavailable
	}
	return proof, nil
}

func read(client *http.Client, req *http.Request, dst any) bool {
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	return err == nil && len(body) <= 65536 && json.Unmarshal(body, dst) == nil
}
