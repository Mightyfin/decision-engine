// Package decisions defines the immutable outcome handed to consuming channels.
package decisions

import "time"

type Outcome string

const (
	// OutcomePendingReview is the only outcome the current automated intake path may create.
	OutcomePendingReview Outcome = "pending_review"
	// These outcomes are recorded only by an authorised Credit Analyst in the current release.
	OutcomeOffered  Outcome = "offered"
	OutcomeDeclined Outcome = "declined"
)

type Decision struct {
	ID, TenantID, SubjectID, ProductPolicyID string
	Outcome                                  Outcome
	ReasonCodes                              []string
	PolicyVersions                           map[string]int
	CreatedAt                                time.Time
}

// ReviewRequest is the tenant-scoped intake contract. It creates a case for a Credit Analyst;
// no automated score, offer, or decline is produced in this release.
type ReviewRequest struct {
	ID, TenantID, SubjectID, ProductPolicyID string
	SubmittedAt                              time.Time
}
