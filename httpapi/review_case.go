package httpapi

import (
	"context"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http"
)

type reviewStore interface {
	Offer(context.Context, string) (creditrisk.Offer, error)
	ReviewHistory(context.Context, string, string) ([]creditrisk.Audit, error)
}

func (s Server) reviewCase(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.principal(w, r, "credit_analyst")
	if !ok {
		return
	}
	version := ""
	if versions, ok := s.Applications.(interface {
		ReviewRevision(context.Context, string, string) (string, error)
	}); ok {
		var err error
		version, err = versions.ReviewRevision(r.Context(), r.PathValue("tenant_id"), r.PathValue("id"))
		if err != nil {
			write(w, 503, map[string]string{"error": "review_unavailable"})
			return
		}
	}
	a, err := s.Applications.Application(r.Context(), r.PathValue("id"))
	if errors.Is(err, creditrisk.ErrNotFound) || (err == nil && a.TenantID != r.PathValue("tenant_id")) {
		write(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		write(w, 503, map[string]string{"error": "review_unavailable"})
		return
	}
	evidenceStatus := "integration_unavailable"
	if reader, available := s.Applications.(evidenceReader); available {
		if principal.Environment == "" {
			write(w, 403, map[string]string{"error": "environment_required"})
			return
		}
		scoped, scopeErr := reader.EvidenceApplication(r.Context(), a.ID)
		if errors.Is(scopeErr, creditrisk.ErrNotFound) || (scopeErr == nil && (scoped.TenantID != a.TenantID || (scoped.Environment != "" && scoped.Environment != principal.Environment))) {
			write(w, 404, map[string]string{"error": "not_found"})
			return
		}
		if scopeErr != nil {
			write(w, 503, map[string]string{"error": "evidence_unavailable"})
			return
		}
		if scoped.Environment == "" {
			evidenceStatus = "application_environment_unknown"
		} else {
			rows, readErr := reader.EvidencePage(r.Context(), a.ID, a.TenantID, principal.Environment, "", "", 1)
			if readErr != nil {
				write(w, 503, map[string]string{"error": "evidence_unavailable"})
				return
			}
			evidenceStatus = "not_linked"
			if len(rows) > 0 {
				evidenceStatus = "linked"
			}
		}
	}
	store, ok := s.Applications.(reviewStore)
	if !ok {
		write(w, 503, map[string]string{"error": "review_unavailable"})
		return
	}
	history, err := store.ReviewHistory(r.Context(), a.TenantID, a.ID)
	if err != nil {
		write(w, 503, map[string]string{"error": "history_unavailable"})
		return
	}
	offer, err := store.Offer(r.Context(), a.ID)
	if err != nil && !errors.Is(err, creditrisk.ErrNotFound) {
		write(w, 503, map[string]string{"error": "offer_unavailable"})
		return
	}
	var offerData any
	if err == nil {
		offerData = offer
	}
	var questionnaire *creditrisk.Questionnaire
	if reader, ok := s.Applications.(interface {
		ReviewQuestionnaire(context.Context, string, string, string) (*creditrisk.Questionnaire, error)
	}); ok {
		questionnaire, err = reader.ReviewQuestionnaire(r.Context(), a.ID, a.TenantID, principal.Environment)
		if errors.Is(err, creditrisk.ErrNotFound) {
			write(w, 404, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			write(w, 503, map[string]string{"error": "questionnaire_unavailable"})
			return
		}
	}
	events := []map[string]any{}
	for _, e := range history {
		events = append(events, map[string]any{"actor": e.Actor, "action": e.Action, "reason": e.Reason, "at": e.At})
	}
	write(w, 200, map[string]any{"application": a, "questionnaire": questionnaire, "review_revision": version, "assessment_context": a.AssessmentContext(), "offer": offerData, "history": events, "document_evidence_status": evidenceStatus})
}
