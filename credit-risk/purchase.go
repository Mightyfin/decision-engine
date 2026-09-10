package creditrisk

import (
	"strings"
	"time"
)

// PurchaseRestriction records the analyst's reviewed destination restriction.
// It is not a Wallet ownership verification, order register or payment command.
type PurchaseRestriction struct {
	DestinationVerification *DestinationVerification `json:"destination_verification,omitempty"`
	ApplicationID           string                   `json:"application_id"`
	OrderReference          string                   `json:"order_reference"`
	SupplierPartyID         string                   `json:"supplier_party_id"`
	DestinationWalletID     string                   `json:"destination_wallet_id"`
	DocumentID              string                   `json:"document_id"`
	SHA256                  string                   `json:"sha256"`
	Currency                string                   `json:"currency"`
	MaximumAmountMinor      int64                    `json:"maximum_amount_minor"`
	RecordedBy              string                   `json:"recorded_by"`
	RecordedAt              time.Time                `json:"recorded_at"`
}

type DestinationVerification struct {
	WalletID      string    `json:"wallet_id"`
	LegalEntityID string    `json:"legal_entity_id"`
	TenantID      string    `json:"tenant_id"`
	PartyID       string    `json:"party_id"`
	Currency      string    `json:"currency"`
	Environment   string    `json:"environment"`
	Verification  string    `json:"verification"`
	VerifiedAt    time.Time `json:"verified_at"`
}

func (p PurchaseRestriction) HasCurrentDestination(tenant, environment string, now time.Time) bool {
	v := p.DestinationVerification
	return v != nil && v.WalletID == p.DestinationWalletID && v.PartyID == p.SupplierPartyID && v.Currency == p.Currency && v.TenantID == tenant && v.Environment == environment && v.LegalEntityID != "" && v.Verification == "active_owned_wallet" && !v.VerifiedAt.After(now.Add(5*time.Second)) && v.VerifiedAt.After(now.Add(-time.Minute))
}

func (p PurchaseRestriction) Validate() error {
	for _, s := range []string{p.ApplicationID, p.OrderReference, p.SupplierPartyID, p.DestinationWalletID, p.DocumentID} {
		if strings.TrimSpace(s) == "" || strings.TrimSpace(s) != s || len(s) > 256 {
			return ErrInvalidState
		}
	}
	if !digestPattern.MatchString(p.SHA256) || p.MaximumAmountMinor < 1 || len(p.Currency) != 3 {
		return ErrInvalidState
	}
	for _, r := range p.Currency {
		if r < 'A' || r > 'Z' {
			return ErrInvalidState
		}
	}
	return nil
}
