package httpapi

import (
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"io"
	"net/http"
	"strings"
)

func (s Server) bindEvidence(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "credit_evidence_writer")
	if !ok {
		return
	}
	if p.TenantID == "" || p.Environment == "" || p.Subject == "" {
		write(w, 403, map[string]string{"error": "evidence_scope_required"})
		return
	}
	store, ok := s.Applications.(creditrisk.EvidenceStore)
	if !ok || s.DocumentURL == "" {
		write(w, 503, map[string]string{"error": "evidence_integration_unavailable"})
		return
	}
	var in struct {
		DocumentID string `json:"document_id"`
		SHA256     string `json:"sha256"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(&struct{}{}) != io.EOF {
		write(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	service := creditrisk.EvidenceService{Store: store, Verifier: creditrisk.HTTPDocumentVerifier{BaseURL: s.DocumentURL, Token: token}}
	err := service.Bind(r.Context(), r.PathValue("id"), p.TenantID, p.Environment, p.Subject, in.DocumentID, in.SHA256)
	if err != nil {
		status, code := 503, "evidence_verification_unavailable"
		if errors.Is(err, creditrisk.ErrNotFound) {
			status, code = 404, "not_found"
		} else if errors.Is(err, creditrisk.ErrEvidenceScope) || errors.Is(err, creditrisk.ErrInvalidState) {
			status, code = 409, "evidence_scope_or_state_conflict"
		}
		write(w, status, map[string]string{"error": code})
		return
	}
	write(w, 200, map[string]string{"status": "evidence_linked", "application_id": r.PathValue("id")})
}
