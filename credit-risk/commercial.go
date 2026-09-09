package creditrisk

import (
	"context"
	"errors"
	"time"
)

var ErrCommercialReviewRequired = errors.New("current application requires tenant commercial approval")

// This is the tenant's business approval, not a lender decision, quote or
// disbursement authority. The reference identifies the tenant's own review.
type CommercialReview struct {
	ApplicationID string    `json:"application_id"`
	Revision      int       `json:"revision"`
	Decision      string    `json:"decision"`
	Reference     string    `json:"review_reference"`
	SnapshotHash  string    `json:"snapshot_hash"`
	RecordedAt    time.Time `json:"recorded_at"`
}
type CommercialReviewStore interface {
	CommercialReview(context.Context, string, SubmissionIdentity) (CommercialReview, error)
	RecordCommercialReview(context.Context, CommercialReview, Audit, SubmissionIdentity) (CommercialReview, bool, error)
}
