package runner

import "github.com/getsentry/sentry-go"

// reportFailure reports a background generation job's failure to Sentry,
// tagged by which runner and which user/job it was. Both runners in this
// package run as detached goroutines (go func() { ... }()) outside any
// HTTP request's lifetime, so there's no per-request hub to pull from the
// way HTTP handlers do (sentry.GetHubFromContext) — this uses the global
// hub directly, same reasoning as the cron jobs in main.go.
func reportFailure(runnerJob, userID, jobID string, err error) {
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("runner_job", runnerJob)
		scope.SetTag("user_id", userID)
		scope.SetTag("job_id", jobID)
		sentry.CaptureException(err)
	})
}
