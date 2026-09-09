package creditrisk

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Mightyfin/decision-engine/product"
)

var ErrDraftRequired = errors.New("product requires draft workflow")
var ErrDraftKeyConflict = errors.New("draft creation key reused with different input")

type creationContextKey struct{}
type DraftCreation struct{ ApplicationID, Hash string }

func WithDraftCreation(ctx context.Context, applicationID, hash string) context.Context {
	return context.WithValue(ctx, creationContextKey{}, DraftCreation{applicationID, hash})
}

var ErrProductChanged = errors.New("product version changed; create a new draft")

type Draft struct {
	CallerApplicationID string                     `json:"-"`
	CreationHash        string                     `json:"-"`
	Application         Application                `json:"application"`
	Requirements        product.Requirements       `json:"requirements"`
	Answers             map[string]json.RawMessage `json:"answers"`
	Revision            int                        `json:"revision"`
}
type DraftStore interface {
	CreateDraft(context.Context, Draft, Audit) error
	GetDraft(context.Context, string, string, string) (Draft, error)
	UpdateDraft(context.Context, Draft, int, Audit, bool) error
	DraftEvidence(context.Context, string, string, string) ([]Evidence, error)
}
type IncompleteError struct{ Completion product.Completion }

func (e IncompleteError) Error() string { return "application incomplete" }

func (s Service) CreateDraft(ctx context.Context, a Application, actor string) (Draft, error) {
	store, ok := s.Store.(DraftStore)
	if !ok {
		return Draft{}, ErrInvalidState
	}
	if a.TenantID == "" || a.Environment == "" || a.PartyID == "" || a.ApplicantRole == "" || actor == "" {
		return Draft{}, ErrEvidenceScope
	}
	creation, _ := ctx.Value(creationContextKey{}).(DraftCreation)
	if creation.Hash != "" {
		existing, err := store.GetDraft(ctx, a.ID, a.TenantID, a.Environment)
		if err == nil {
			if existing.CallerApplicationID != creation.ApplicationID || existing.CreationHash != creation.Hash {
				return Draft{}, ErrDraftKeyConflict
			}
			return existing, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return Draft{}, err
		}
	}
	a, p, err := s.prepareApplication(ctx, a)
	if err != nil {
		return Draft{}, err
	}
	a.Status = "draft"
	d := Draft{Application: a, Requirements: p.RequirementsFor(a.ApplicantRole), Answers: map[string]json.RawMessage{}, Revision: 1}
	d.CallerApplicationID = creation.ApplicationID
	d.CreationHash = creation.Hash
	if err = store.CreateDraft(ctx, d, Audit{ApplicationID: a.ID, Actor: actor, Action: "draft_created", Reason: "application draft created", At: s.now()}); err != nil {
		return Draft{}, err
	}
	return store.GetDraft(ctx, a.ID, a.TenantID, a.Environment)
}

// SaveDraft replaces answers under optimistic concurrency. Financial terms and
// applicant identity are fixed for this draft; changes require a new draft.
func (s Service) SaveDraft(ctx context.Context, id, tenant, environment, actor string, revision int, answers map[string]json.RawMessage) (Draft, error) {
	store, ok := s.Store.(DraftStore)
	if !ok {
		return Draft{}, ErrInvalidState
	}
	d, err := store.GetDraft(ctx, id, tenant, environment)
	if err != nil {
		return Draft{}, err
	}
	if actor == "" || (d.Application.Status != "draft" && d.Application.Status != "awaiting_information") || revision != d.Revision {
		return Draft{}, ErrInvalidState
	}
	c := d.Requirements.Check(answers, nil)
	c.MissingDocuments = []string{}
	if len(c.InvalidFields) > 0 || (d.Application.Status == "awaiting_information" && len(c.MissingFields) > 0) {
		return Draft{}, IncompleteError{c}
	}
	if answers == nil {
		answers = map[string]json.RawMessage{}
	}
	d.Answers = answers
	d.Revision++
	return d, store.UpdateDraft(ctx, d, revision, Audit{ApplicationID: id, Actor: actor, Action: "draft_updated", Reason: "application answers updated", At: s.now()}, false)
}

func (s Service) SubmitDraft(ctx context.Context, id, tenant, environment, actor string, revision int, verifier EvidenceVerifier) (Draft, error) {
	store, ok := s.Store.(DraftStore)
	if !ok {
		return Draft{}, ErrInvalidState
	}
	d, err := store.GetDraft(ctx, id, tenant, environment)
	if err != nil {
		return Draft{}, err
	}
	if actor == "" || d.Application.Status != "draft" || revision != d.Revision {
		return Draft{}, ErrInvalidState
	}
	a := d.Application
	p, err := s.Products.Validate(ctx, tenant, a.ProductPolicyID, a.Currency, a.Amount, a.TermDays, a.ApplicantRole)
	if err != nil {
		return Draft{}, err
	}
	if p.Version != a.ProductPolicyVersion {
		return Draft{}, ErrProductChanged
	}
	documents := []string{}
	if len(d.Requirements.RequiredDocumentTypes) > 0 {
		if verifier == nil {
			return Draft{}, ErrEvidenceScope
		}
		evidence, err := store.DraftEvidence(ctx, id, tenant, environment)
		if err != nil {
			return Draft{}, err
		}
		for _, e := range evidence {
			kind, err := verifier.VerifyEvidence(ctx, a, e.DocumentID, e.SHA256)
			if err != nil {
				return Draft{}, err
			}
			if kind == "" || kind != e.DocumentType {
				return Draft{}, ErrEvidenceScope
			}
			documents = append(documents, kind)
		}
	}
	c := d.Requirements.Check(d.Answers, documents)
	if !c.Complete {
		return Draft{}, IncompleteError{c}
	}
	if d.Requirements.CommercialReviewRequired {
		reviews, ok := s.Store.(CommercialReviewStore)
		if !ok {
			return Draft{}, ErrCommercialReviewRequired
		}
		review, err := reviews.CommercialReview(ctx, id, SubmissionIdentity{TenantID: tenant, Environment: environment, CallerApplicationID: d.CallerApplicationID})
		if err != nil {
			return Draft{}, err
		}
		if review.Decision != "approved" || review.Revision != revision {
			return Draft{}, ErrCommercialReviewRequired
		}
	}
	d.Application.Status = "pending_review"
	d.Application.SubmittedAt = s.now()
	d.Revision++
	return d, store.UpdateDraft(ctx, d, revision, Audit{ApplicationID: id, Actor: actor, Action: "submitted", Reason: "configured application requirements complete; manual review required", At: s.now()}, true)
}
