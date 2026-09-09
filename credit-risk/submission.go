package creditrisk

import (
	"context"
	"errors"
)

var ErrSubmissionUnavailable = errors.New("durable submission unavailable")

type SubmissionIdentity struct{ TenantID, Environment, CallerApplicationID, Key, Hash string }
type submissionContextKey struct{}

func WithSubmissionIdentity(ctx context.Context, in SubmissionIdentity) context.Context {
	return context.WithValue(ctx, submissionContextKey{}, in)
}

type SubmissionStore interface {
	SubmissionReplay(context.Context, SubmissionIdentity) (Application, bool, error)
	CreateSubmission(context.Context, Application, Audit, SubmissionIdentity) (Application, error)
}
