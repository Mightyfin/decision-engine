// Package decisions defines the immutable outcome handed to consuming channels.
package decisions

import "time"

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
