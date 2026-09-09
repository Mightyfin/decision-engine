package httpapi

import (
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/product"
	"io"
	"net/http"
	"strings"
)

func (s Server) draft(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "decision_workload")
	if !ok {
		return
	}
	if p.TenantID == "" || p.Environment == "" || p.Subject == "" {
		write(w, 403, map[string]string{"error": "scope_required"})
		return
	}
	tenantEditor := p.ApplicationID == "" && p.Roles["credit_evidence_writer"] && (p.Roles["tenant_owner"] || p.Roles["tenant_admin"] || p.Roles["tenant_credit_operator"])
	if r.Method != "GET" && !p.Roles["credit_application_writer"] && !tenantEditor {
		write(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	store, ok := s.Credit.Store.(creditrisk.DraftStore)
	if !ok {
		write(w, 503, map[string]string{"error": "drafts_unavailable"})
		return
	}
	id := r.PathValue("id")
	if r.Method == "GET" {
		d, err := store.GetDraft(r.Context(), id, p.TenantID, p.Environment)
		if err != nil {
			draftError(w, err)
			return
		}
		write(w, 200, d)
		return
	}
	var in struct {
		Revision int                        `json:"revision"`
		Answers  map[string]json.RawMessage `json:"answers"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(&struct{}{}) != io.EOF || in.Revision < 1 {
		write(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	ctx := product.WithBearerToken(r.Context(), token)
	var d creditrisk.Draft
	var err error
	if r.Method == "PUT" {
		d, err = s.Credit.SaveDraft(ctx, id, p.TenantID, p.Environment, p.Subject, in.Revision, in.Answers)
	} else {
		// Submit never trusts answers or document flags in the submit request.
		if in.Answers != nil {
			write(w, 400, map[string]string{"error": "save_answers_before_submission"})
			return
		}
		var verifier creditrisk.EvidenceVerifier
		if s.DocumentURL != "" {
			verifier = creditrisk.HTTPDocumentVerifier{BaseURL: s.DocumentURL, Token: token}
		}
		d, err = s.Credit.SubmitDraft(ctx, id, p.TenantID, p.Environment, p.Subject, in.Revision, verifier)
	}
	if err != nil {
		draftError(w, err)
		return
	}
	write(w, 200, d)
}
func draftError(w http.ResponseWriter, err error) {
	var incomplete creditrisk.IncompleteError
	if errors.As(err, &incomplete) {
		write(w, 422, map[string]any{"error": "application_incomplete", "completion": incomplete.Completion})
		return
	}
	status, code := 422, "validation_failed"
	switch {
	case errors.Is(err, creditrisk.ErrDraftKeyConflict):
		status, code = 409, "idempotency_conflict"
	case errors.Is(err, creditrisk.ErrDraftRequired):
		code = "draft_required"
	case errors.Is(err, creditrisk.ErrProductChanged):
		status, code = 409, "product_version_changed"
	case errors.Is(err, creditrisk.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, creditrisk.ErrInvalidState):
		status, code = 409, "state_or_revision_conflict"
	case errors.Is(err, creditrisk.ErrEvidenceScope):
		status, code = 409, "evidence_scope_required"
	}
	write(w, status, map[string]string{"error": code})
}
