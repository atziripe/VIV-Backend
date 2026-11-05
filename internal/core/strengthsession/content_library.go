package strengthsession

import (
	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/strengthlibrary"
)

// ContentLibraryAdapter implements cascade.ContentLibrary — wired as
// cascade.DefaultContentLibrary so the cascade's ReduceDuration lever
// can actually fire for Strength. Before this, DefaultContentLibrary
// was never assigned anywhere (still the package's own always-false
// stub), so ReduceDuration/SimplifyComplexity were dead code for every
// activity type, Strength included.
//
// Enabled gates the WHOLE adapter, not just the dispatch into the new
// assembly path — this must match usecase.SessionContentSelector's own
// feature flag exactly. If HasVariant(Strength, Short) returned true
// while the OLD content-selection path (content.ExerciseLibrary
// hydration) were still active, the cascade could assign
// DurationTier=Short to a Strength slot that the old path has no idea
// how to handle (it already hard-errors on any non-Standard
// DurationTier) — a real regression for "every other activity type
// must keep being served exactly as it is today," since with the flag
// off, Strength itself must also keep behaving exactly as today: HasVariant
// always false, like the stub it replaces.
type ContentLibraryAdapter struct {
	Config  *strengthlibrary.BlockConfig
	Enabled bool
}

var _ cascade.ContentLibrary = ContentLibraryAdapter{}

// HasVariant implements cascade.ContentLibrary.
//
//   - Any activity type other than Strength: always false — "exactly
//     what it returned before" (the stub), unconditionally, flag or no
//     flag; this adapter never touches that path.
//   - Strength + VariantShort: true whenever Enabled — a short variant
//     genuinely exists (DurationTier=Short's slot-dropping behavior,
//     see duration.go).
//   - Strength + VariantSimplified: Config.VariantRules().SimplifiedActive
//     when Enabled, read from the config's own variant_rules.simplified.active
//     rather than hardcoded — "not available... do not fake it" means
//     reporting the real value, not a Go-level true/false the config
//     can't override later once ratings are reviewed.
func (a ContentLibraryAdapter) HasVariant(id activity.ID, v cascade.Variant) bool {
	if !a.Enabled || id != activity.Strength {
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
