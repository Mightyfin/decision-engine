package creditrisk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Token is the caller's delegated, tenant/environment-scoped credential, never
// an unrestricted document-service account. Construct per request.
type HTTPDocumentVerifier struct {
	BaseURL, Token string
	Client         *http.Client
}

func (v HTTPDocumentVerifier) VerifyEvidence(ctx context.Context, a Application, id, digest string) (string, error) {
	if v.BaseURL == "" || v.Token == "" {
		return "", ErrEvidenceScope
	}
	body, _ := json.Marshal(map[string]string{"party_id": a.PartyID, "sha256": digest})
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(v.BaseURL, "/")+"/v1/documents/"+url.PathEscape(id)+"/evidence-verification", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+v.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-Id", a.ID)
	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	// Never follow an upstream redirect carrying a delegated credential.
	safe := *client
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := safe.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("document verification unavailable (%d)", res.StatusCode)
	}
	var out struct {
		TenantID     string `json:"tenant_id"`
		Environment  string `json:"environment"`
		Verification string `json:"verification"`
		Document     struct {
			ID         string `json:"id"`
			PartyID    string `json:"party_id"`
			OwnerType  string `json:"owner_type"`
			OwnerID    string `json:"owner_id"`
			SHA256     string `json:"sha256"`
			Status     string `json:"status"`
			ScanStatus string `json:"scan_status"`
			Type       string `json:"document_type"`
		} `json:"document"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	d := out.Document
	if out.TenantID != a.TenantID || out.Environment != a.Environment || out.Verification != "available_clean_version" || d.ID != id || d.PartyID != a.PartyID || d.OwnerType != "PARTY" || d.OwnerID != a.PartyID || d.SHA256 != digest || d.Status != "available" || d.ScanStatus != "clean" || d.Type == "" {
		return "", ErrEvidenceScope
	}
	return d.Type, nil
}
