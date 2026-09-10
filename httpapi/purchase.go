package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"io"
	"net/http"
	"time"
)

type purchaseStore interface {
	PurchaseRestriction(context.Context, string, string, string) (creditrisk.PurchaseRestriction, error)
	RecordPurchaseRestriction(context.Context, creditrisk.PurchaseRestriction, string, string, string, string, string) error
}

func (s Server) purchaseRestriction(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "credit_analyst")
	if !ok {
		return
	}
	if p.TenantID != "" || p.Environment == "" || p.Subject == "" {
		write(w, 403, map[string]string{"error": "staff_scope_required"})
		return
	}
	store, ok := s.Applications.(purchaseStore)
	if !ok {
		write(w, 503, map[string]string{"error": "purchase_review_unavailable"})
		return
	}
	id, tenant := r.PathValue("id"), r.PathValue("tenant_id")
	if r.Method == "POST" {
		var in struct {
			OrderReference      string `json:"order_reference"`
			SupplierPartyID     string `json:"supplier_party_id"`
			DestinationWalletID string `json:"destination_wallet_id"`
			DocumentID          string `json:"document_id"`
			SHA256              string `json:"sha256"`
			Currency            string `json:"currency"`
			MaximumAmountMinor  int64  `json:"maximum_amount_minor"`
			ReviewRevision      string `json:"review_revision"`
			Reason              string `json:"reason"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		d.DisallowUnknownFields()
		if d.Decode(&in) != nil || d.Decode(&struct{}{}) != io.EOF {
			write(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		restriction := creditrisk.PurchaseRestriction{ApplicationID: id, OrderReference: in.OrderReference, SupplierPartyID: in.SupplierPartyID, DestinationWalletID: in.DestinationWalletID, DocumentID: in.DocumentID, SHA256: in.SHA256, Currency: in.Currency, MaximumAmountMinor: in.MaximumAmountMinor}
		if restriction.Validate() != nil {
			write(w, 400, map[string]string{"error": "invalid_purchase_restriction"})
			return
		}
		if s.DestinationVerifier == nil {
			write(w, 503, map[string]string{"error": "destination_verification_unavailable"})
			return
		}
		proof, err := s.DestinationVerifier.Verify(r.Context(), tenant, restriction)
		if err != nil {
			write(w, 503, map[string]string{"error": "destination_verification_unavailable"})
			return
		}
		restriction.DestinationVerification = &proof
		if !restriction.HasCurrentDestination(tenant, p.Environment, time.Now().UTC()) {
			write(w, 503, map[string]string{"error": "destination_verification_unavailable"})
			return
		}
		if err := store.RecordPurchaseRestriction(r.Context(), restriction, tenant, p.Environment, p.Subject, in.Reason, in.ReviewRevision); err != nil {
			purchaseError(w, err)
			return
		}
	}
	item, err := store.PurchaseRestriction(r.Context(), id, tenant, p.Environment)
	if err != nil {
		purchaseError(w, err)
		return
	}
	write(w, 200, item)
}
func purchaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, creditrisk.ErrNotFound):
		write(w, 404, map[string]string{"error": "not_found"})
	case errors.Is(err, creditrisk.ErrInvalidState), errors.Is(err, creditrisk.ErrEvidenceScope):
		write(w, 409, map[string]string{"error": "purchase_review_conflict"})
	default:
		write(w, 503, map[string]string{"error": "purchase_review_unavailable"})
	}
}
