package httpapi

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http"
)

type evidenceReader interface {
	EvidenceApplication(context.Context, string) (creditrisk.Application, error)
	EvidencePage(context.Context, string, string, string, string, string, int) ([]creditrisk.Evidence, error)
}

func (s Server) listEvidence(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "credit_analyst")
	if !ok {
		return
	}
	if p.Environment == "" {
		write(w, 403, map[string]string{"error": "environment_required"})
		return
	}
	store, ok := s.Applications.(evidenceReader)
	if !ok {
		write(w, 503, map[string]string{"error": "evidence_unavailable"})
		return
	}
	a, err := store.EvidenceApplication(r.Context(), r.PathValue("id"))
	if errors.Is(err, creditrisk.ErrNotFound) || (err == nil && (a.TenantID != r.PathValue("tenant_id") || (a.Environment != "" && a.Environment != p.Environment))) {
		write(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		write(w, 503, map[string]string{"error": "evidence_unavailable"})
		return
	}
	if a.Environment == "" {
		write(w, 409, map[string]string{"error": "application_environment_unknown"})
		return
	}
	doc, hash := r.URL.Query().Get("after_document"), r.URL.Query().Get("after_sha256")
	if (doc == "") != (hash == "") || len(doc) > 256 || len(hash) > 64 {
		write(w, 400, map[string]string{"error": "invalid_cursor"})
		return
	}
	rows, err := store.EvidencePage(r.Context(), a.ID, a.TenantID, a.Environment, doc, hash, 51)
	if err != nil {
		write(w, 503, map[string]string{"error": "evidence_unavailable"})
		return
	}
	nextDoc, nextHash := "", ""
	if len(rows) > 50 {
		rows = rows[:50]
		last := rows[len(rows)-1]
		nextDoc, nextHash = last.DocumentID, last.SHA256
	}
	write(w, 200, map[string]any{"data": rows, "next_document": nextDoc, "next_sha256": nextHash, "environment": a.Environment})
}
