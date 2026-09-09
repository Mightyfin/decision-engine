package creditrisk

import "time"

// TimelineEvent is deliberately separate from internal Audit. It cannot expose
// analyst identity, free-text reasons, evidence contents or internal risk rules.
type TimelineEvent struct {
	ID     string    `json:"id"`
	Action string    `json:"action"`
	At     time.Time `json:"at"`
}
