// Package modelscoring defines the controlled boundary for algorithmic underwriting models.
package modelscoring

import "context"

// Input contains only approved, tenant-scoped features. Raw source documents stay in their
// source systems; provenance is retained by the feature-generation implementation.
type Input struct {
	TenantID, SubjectID string
	Features            map[string]float64
}

type Result struct {
	ModelID, ModelVersion string
	Score                 float64
	ReasonCodes           []string
}

// Scorer is implemented by a versioned model adapter. It must return explainable reason codes;
// it cannot write decisions, pricing, applications, ledgers, or payment instructions.
type Scorer interface {
	Score(context.Context, Input) (Result, error)
}
