package creditrisk

import "context"

type CancellationStore interface {
	CancelApplication(context.Context, Application, Audit, SubmissionIdentity) (Application, bool, error)
}
