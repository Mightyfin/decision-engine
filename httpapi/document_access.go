package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"io"
	"net/http"
)

type documentAccessStore interface {
	EvidenceApplication(context.Context, string) (creditrisk.Application, error)
	HasEvidence(context.Context, string, string, string, string, string) (bool, error)
}

// Document Service calls this with the original signed caller credential. It
// must not substitute a broadly privileged service account.
func (s Server) documentAccess(w http.ResponseWriter, r *http.Request) {
	p, err := s.Auth.Authenticate(r)
	if err != nil || p.Subject == "" || p.Environment == "" {
		write(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var in struct {
		TenantID   string `json:"tenant_id"`
		Intent     string `json:"intent"`
		DocumentID string `json:"document_id"`
		SHA256     string `json:"sha256"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(&struct{}{}) != io.EOF || in.TenantID == "" {
		write(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	staffRead := in.Intent == "download" && p.Roles["credit_analyst"]
	tenantWrite := p.TenantID == in.TenantID && p.Roles["credit_evidence_writer"]
	tenantRead := p.TenantID == in.TenantID && (p.Roles["decision_workload"] || tenantWrite)
	if (in.Intent == "download" && !staffRead && !tenantRead) || (in.Intent == "upload" && !tenantWrite) || (in.Intent == "uploads" && !tenantRead) || (in.Intent != "download" && in.Intent != "upload" && in.Intent != "uploads") {
		write(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	store, ok := s.Applications.(documentAccessStore)
	if !ok {
		write(w, 503, map[string]string{"error": "document_access_unavailable"})
		return
	}
	a, err := store.EvidenceApplication(r.Context(), r.PathValue("id"))
	if errors.Is(err, creditrisk.ErrNotFound) || (err == nil && (a.TenantID != in.TenantID || (a.Environment != "" && a.Environment != p.Environment))) {
		write(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		write(w, 503, map[string]string{"error": "document_access_unavailable"})
		return
	}
	if a.Environment == "" || a.PartyID == "" || a.RelationshipID == "" || a.AssessmentContext().Scenario == "unclassified" {
		write(w, 409, map[string]string{"error": "case_scope_unverified"})
		return
	}
	if in.Intent == "upload" && a.Status != "pending_review" && a.Status != "awaiting_information" {
		write(w, 409, map[string]string{"error": "case_not_accepting_uploads"})
		return
	}
	if in.Intent == "download" {
		found, err := store.HasEvidence(r.Context(), a.ID, a.TenantID, a.Environment, in.DocumentID, in.SHA256)
		if err != nil {
			write(w, 503, map[string]string{"error": "document_access_unavailable"})
			return
		}
		if !found {
			write(w, 404, map[string]string{"error": "not_found"})
			return
		}
	}
	write(w, 200, map[string]string{"application_id": a.ID, "tenant_id": a.TenantID, "environment": a.Environment, "party_id": a.PartyID, "intent": in.Intent, "document_id": in.DocumentID, "sha256": in.SHA256, "actor": p.Subject})
}
