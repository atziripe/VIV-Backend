package strengthsession

import (
	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/strengthlibrary"
)

// UserRollout is the per-user decision ContentLibraryAdapter needs —
// exactly usecase.StrengthSessionsRollout's shape, defined here instead
// of imported to avoid a cycle (usecase already imports strengthsession
// for the real assembly path): main.go passes a
// usecase.StrengthSessionsRollout value in directly, which satisfies
// this interface structurally.
type UserRollout interface {
	Enabled(userID string) bool
}

// ContentLibraryAdapter implements cascade.ContentLibrary — wired as
// cascade.DefaultContentLibrary so the cascade's ReduceDuration lever
// can actually fire for Strength. Before this, DefaultContentLibrary
// was never assigned anywhere (still the package's own always-false
// stub), so ReduceDuration/SimplifyComplexity were dead code for every
// activity type, Strength included.
//
// Rollout gates the WHOLE adapter, not just the dispatch into the new
// assembly path — this must match usecase.SessionContentSelector's own
// per-user rollout decision exactly (same Rollout value, wired from the
// same usecase.StrengthSessionsRollout in main.go), evaluated for the
// SAME userID (threaded through via cascade.UserCatalog.UserID ->
// ResolveSlotConflict -> HasVariant's userID parameter). If
// HasVariant(userID, Strength, Short) ever returned true for a user the
// rollout doesn't cover, the cascade could assign DurationTier=Short to
// their Strength slot, and the OLD content-selection path
// (content.ExerciseLibrary hydration) has no idea how to handle that —
// it already hard-errors on any non-Standard DurationTier. Keeping
// Rollout.Enabled(userID) as the single source of truth for BOTH the
// cascade adapter and the content-selection dispatch is what prevents
// that split-brain.
type ContentLibraryAdapter struct {
	Config  *strengthlibrary.BlockConfig
	Rollout UserRollout
}

var _ cascade.ContentLibrary = ContentLibraryAdapter{}

// HasVariant implements cascade.ContentLibrary.
//
//   - Any activity type other than Strength: always false — "exactly
//     what it returned before" (the stub), unconditionally; this
//     adapter never touches that path.
//   - Strength + VariantShort: true whenever Rollout.Enabled(userID) —
//     a short variant genuinely exists for this user (DurationTier=Short's
//     slot-dropping behavior, see duration.go).
//   - Strength + VariantSimplified: Config.VariantRules().SimplifiedActive
//     when Rollout.Enabled(userID), read from the config's own
//     variant_rules.simplified.active rather than hardcoded — "not
//     available... do not fake it" means reporting the real value, not
//     a Go-level true/false the config can't override later once
//     ratings are reviewed.
func (a ContentLibraryAdapter) HasVariant(userID string, id activity.ID, v cascade.Variant) bool {
	if a.Rollout == nil || !a.Rollout.Enabled(userID) || id != activity.Strength {
		return false
	}
	switch v {
	case cascade.VariantShort:
		return true
	case cascade.VariantSimplified:
		return a.Config.VariantRules().SimplifiedActive
	default:
		return false
	}
}
