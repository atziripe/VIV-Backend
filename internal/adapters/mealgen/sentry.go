package mealgen

import "github.com/getsentry/sentry-go"

// reportFailure reports a background copy-enrichment failure to Sentry.
// Both enrichers in this package run as detached goroutines outside any
// HTTP request's lifetime, so there's no per-request hub to pull from —
// this uses the global hub directly, same reasoning as
// internal/adapters/runner's own reportFailure.
func reportFailure(job, userID string, err error) {
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("runner_job", job)
		scope.SetTag("user_id", userID)
		sentry.CaptureException(err)
	})
}
