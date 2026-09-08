package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"io"
	"net/http"
	"strings"
)

type informationStore interface {
	EvidenceApplication(context.Context, string) (creditrisk.Application, error)
	ReviewRevision(context.Context, string, string) (string, error)
	ChangeInformationState(context.Context, string, string, string, string, string, string, bool) error
	InformationMessage(context.Context, string, string) (string, error)
}

func (s Server) information(w http.ResponseWriter, r *http.Request) {
	staff := r.PathValue("tenant_id") != ""
	role := "credit_evidence_writer"
	if staff {
		role = "credit_analyst"
	}
	if r.Method == "GET" {
		role = "decision_workload"
	}
	p, ok := s.principal(w, r, role)
	if !ok {
		return
	}
	tenant := p.TenantID
	if staff {
		tenant = r.PathValue("tenant_id")
	}
	if tenant == "" || p.Environment == "" || p.Subject == "" {
		write(w, 403, map[string]string{"error": "scope_required"})
		return
	}
	store, ok := s.Applications.(informationStore)
	if !ok {
		write(w, 503, map[string]string{"error": "workflow_unavailable"})
		return
	}
	id := r.PathValue("id")
	// Capture the version before reading content; concurrent changes make it stale.
	revision, err := store.ReviewRevision(r.Context(), tenant, id)
	if err != nil {
		write(w, 503, map[string]string{"error": "workflow_unavailable"})
		return
	}
	a, err := store.EvidenceApplication(r.Context(), id)
	if errors.Is(err, creditrisk.ErrNotFound) || (err == nil && (a.TenantID != tenant || (a.Environment != "" && a.Environment != p.Environment))) {
		write(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		write(w, 503, map[string]string{"error": "workflow_unavailable"})
		return
	}
	if a.Environment == "" {
		write(w, 409, map[string]string{"error": "application_environment_unknown"})
		return
	}
	if r.Method == "GET" {
		message, err := store.InformationMessage(r.Context(), tenant, id)
		if err != nil {
			write(w, 503, map[string]string{"error": "workflow_unavailable"})
			return
		}
		write(w, 200, map[string]string{"application_id": id, "status": a.Status, "review_revision": revision, "information_requested": message})
		return
	}
	var in struct {
		Reason   string `json:"reason"`
		Revision string `json:"review_revision"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 8192))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(&struct{}{}) != io.EOF || len(strings.TrimSpace(in.Reason)) < 10 || len(in.Reason) > 4000 || in.Revision == "" {
		write(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	err = store.ChangeInformationState(r.Context(), id, tenant, p.Environment, p.Subject, in.Reason, in.Revision, !staff)
	if err != nil {
		status := 503
		if errors.Is(err, creditrisk.ErrInvalidState) {
			status = 409
		}
		if errors.Is(err, creditrisk.ErrNotFound) {
			status = 404
		}
		write(w, status, map[string]string{"error": "workflow_changed_or_unavailable"})
		return
	}
	status := "awaiting_information"
	if !staff {
		status = "pending_review"
	}
	write(w, 200, map[string]string{"application_id": id, "status": status})
}
