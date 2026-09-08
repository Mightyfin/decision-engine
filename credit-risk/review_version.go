package creditrisk

import "context"

type reviewVersionKey struct{}

func WithReviewVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, reviewVersionKey{}, version)
}
func ReviewVersion(ctx context.Context) string {
	v, _ := ctx.Value(reviewVersionKey{}).(string)
	return v
}
