package creditrisk

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var ErrEvidenceScope = errors.New("application evidence scope unavailable")
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Evidence struct {
	ApplicationID string    `json:"application_id"`
	DocumentID    string    `json:"document_id"`
	SHA256        string    `json:"sha256"`
	TenantID      string    `json:"tenant_id"`
	Environment   string    `json:"environment"`
	PartyID       string    `json:"party_id"`
	DocumentType  string    `json:"document_type"`
	LinkedBy      string    `json:"linked_by"`
	LinkedAt      time.Time `json:"linked_at"`
}
type EvidenceStore interface {
	EvidenceApplication(context.Context, string) (Application, error)
	BindEvidence(context.Context, Evidence) error
}

// Verifier must check tenant, environment, subject, digest and clean scan through
// the document owner. Never accept successful verification from a browser.
type EvidenceVerifier interface {
	VerifyEvidence(context.Context, Application, string, string) (string, error)
}
type EvidenceService struct {
	Store    EvidenceStore
	Verifier EvidenceVerifier
}

func (s EvidenceService) Bind(ctx context.Context, id, tenant, environment, actor, document, digest string) error {
	if tenant == "" || environment == "" || actor == "" || document == "" || !digestPattern.MatchString(digest) {
		return ErrEvidenceScope
	}
	a, err := s.Store.EvidenceApplication(ctx, id)
	if err != nil {
		return err
	}
	if a.TenantID != tenant {
		return ErrNotFound
	}
	if a.Environment == "" || a.Environment != environment || a.PartyID == "" || a.RelationshipID == "" || a.AssessmentContext().Scenario == "unclassified" {
		return ErrEvidenceScope
	}
	if a.Status != "draft" && a.Status != "pending_review" && a.Status != "awaiting_information" {
		return ErrInvalidState
	}
	if s.Verifier == nil {
		return ErrEvidenceScope
	}
	kind, err := s.Verifier.VerifyEvidence(ctx, a, document, digest)
	if err != nil {
		return err
	}
	if kind == "" {
		return ErrEvidenceScope
	}
	return s.Store.BindEvidence(ctx, Evidence{ApplicationID: id, DocumentID: document, SHA256: digest, TenantID: tenant, Environment: environment, PartyID: a.PartyID, DocumentType: kind, LinkedBy: actor})
}
