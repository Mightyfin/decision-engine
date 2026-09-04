// Package decisions defines the immutable outcome handed to consuming channels.
package decisions

import (
	"context"
	"fmt"
	"time"

	"github.com/Mightyfin/decision-engine/intelligence/model-scoring"
	"github.com/Mightyfin/decision-engine/policies"
)

type Outcome string

const (
	OutcomeOffered  Outcome = "offered"
	OutcomeDeclined Outcome = "declined"
	OutcomeReferred Outcome = "referred"
)

type Decision struct {
	ID, TenantID, SubjectID, ProductPolicyID string
	Outcome                                  Outcome
	ReasonCodes                              []string
	PolicyVersions                           map[string]int
	CreatedAt                                time.Time
}

type EvaluateInput struct {
	ID, TenantID, SubjectID, ProductPolicyID string
	Features                                 map[string]float64
}

type Evaluator struct {
	Scorer  modelscoring.Scorer
	Routing policies.CreditRouting
	Clock   func() time.Time
}

// Evaluate is the path used by EFaaS and embedded-finance channels. It makes a policy-routed,
// explainable algorithmic decision. Analysts do not approve normal traffic; they receive only
// OutcomeReferred cases and may later perform a separately audited override.
func (e Evaluator) Evaluate(ctx context.Context, in EvaluateInput) (Decision, error) {
	if e.Scorer == nil || !e.Routing.Valid() || in.ID == "" || in.TenantID == "" || in.SubjectID == "" || in.ProductPolicyID == "" || in.TenantID != e.Routing.TenantID {
		return Decision{}, fmt.Errorf("invalid evaluation request or routing policy")
	}
	result, err := e.Scorer.Score(ctx, modelscoring.Input{TenantID: in.TenantID, SubjectID: in.SubjectID, Features: in.Features})
	if err != nil || result.ModelID == "" || result.ModelVersion == "" {
		return Decision{}, fmt.Errorf("algorithmic score unavailable")
	}
	outcome := OutcomeReferred
	if result.Score >= e.Routing.AutoOfferMinimum {
		outcome = OutcomeOffered
	} else if result.Score <= e.Routing.AutoDeclineMaximum {
		outcome = OutcomeDeclined
	}
	now := time.Now().UTC()
	if e.Clock != nil {
		now = e.Clock().UTC()
	}
	return Decision{ID: in.ID, TenantID: in.TenantID, SubjectID: in.SubjectID, ProductPolicyID: in.ProductPolicyID, Outcome: outcome, ReasonCodes: result.ReasonCodes, PolicyVersions: map[string]int{"credit-routing": e.Routing.Version}, CreatedAt: now}, nil
}
