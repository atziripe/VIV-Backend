package domain

// PendingDeletionReason records why a user's account has a deletion
// scheduled (see User.PendingDeletionAt), because the two sources treat
// "the user came back" differently:
//   - PendingDeletionUserRequested: the user asked for this themselves
//     (DELETE /me). Only an explicit cancel clears it — ordinary app usage
//     during the grace period does not, since the request was deliberate.
//   - PendingDeletionInactivity: the system scheduled this after 2 years of
//     no activity and a re-engagement email (see RunInactivitySweepUseCase).
//     Any authenticated request during the notice window auto-cancels it
//     (see RecordUserActivityUseCase) — the whole point of the email was
//     "are you still there?", and a login answers that question.
type PendingDeletionReason string

const (
	PendingDeletionUserRequested PendingDeletionReason = "user_requested"
	PendingDeletionInactivity    PendingDeletionReason = "inactivity"
)
