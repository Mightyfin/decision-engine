package decisions

import (
	"context"
	"testing"

	"github.com/Mightyfin/decision-engine/intelligence/model-scoring"
	"github.com/Mightyfin/decision-engine/policies"
)

type scorer struct{ score float64 }

func (s scorer) Score(context.Context, modelscoring.Input) (modelscoring.Result, error) {
	return modelscoring.Result{ModelID: "risk-v1", ModelVersion: "2026.09", Score: s.score, ReasonCodes: []string{"verified_cashflow"}}, nil
}

func TestAlgorithmicRoutingUsesConfiguredPolicy(t *testing.T) {
	e := Evaluator{Scorer: scorer{score: 0.82}, Routing: policies.CreditRouting{PolicyID: "routing", TenantID: "ten", Version: 3, AutoDeclineMaximum: 0.30, AutoOfferMinimum: 0.70, Active: true}}
	d, err := e.Evaluate(context.Background(), EvaluateInput{ID: "dec_1", TenantID: "ten", SubjectID: "rel_1", ProductPolicyID: "prd_1"})
	if err != nil || d.Outcome != OutcomeOffered || d.PolicyVersions["credit-routing"] != 3 {
		t.Fatal(d, err)
	}
}
