// Package strengthlibrary is VIV-101's reviewed Strength movement catalog
// — a separate, new library living next to the existing content system
// (internal/core/content, internal/core/mesocycle), which this package
// does not modify, read from, or depend on. The old session/exercise
// libraries keep loading and working exactly as before.
//
// This package answers a narrower question than content.ExerciseLibrary
// does: "what Strength movements exist, and has a sport scientist signed
// off on them" — not "what sets/reps/rest should today's session use."
// Sets, reps, load and session generation stay out of scope here; they
// are content.Exercise's job (internal/core/content/exercise_library.go)
// and VIV-108/112's, once that layer is ready to consume this library's
// approved set. Explored content.Exercise before writing this: it
// already uses the name "Exercise" for a different concept (a
// mesocycle-pinned exercise WITH a prescribed sets/reps/rest), so this
// package's Exercise is deliberately its own type in its own package —
// same name is fine across packages, it would not be fine colliding
// inside the same one.
package strengthlibrary

import "viv/internal/core/activity"

// Zone is the body region a movement belongs to — VIV-101 calls this
// "primary classification" via activity.MuscleGroup; Zone is this
// package's own short-form enum for the same four values, used as the
// exercise ID's prefix (see exerciseID). MuscleGroup converts a Zone to
// the activity.MuscleGroup value a future consumer (VIV-108) would need
// to match against a resolved slot.
type Zone string

const (
	ZoneLower Zone = "lower"
	ZoneUpper Zone = "upper"
	ZoneFull  Zone = "full"
	ZoneCore  Zone = "core"
)

// AllZones lists every valid Zone, in a fixed order — used by the
// importer to validate a workbook tab name and to iterate deterministically.
var AllZones = []Zone{ZoneLower, ZoneUpper, ZoneFull, ZoneCore}

// MuscleGroup maps this package's Zone to VIV-101's activity.MuscleGroup.
// Returns "" for an invalid Zone — callers that need a hard guarantee
// should validate the Zone first (the loader does, at startup).
func (z Zone) MuscleGroup() activity.MuscleGroup {
	switch z {
	case ZoneLower:
		return activity.MuscleGroupLower
	case ZoneUpper:
		return activity.MuscleGroupUpper
	case ZoneFull:
		return activity.MuscleGroupFullBody
	case ZoneCore:
		return activity.MuscleGroupCore
	default:
		return ""
	}
}

// MovementGroup is the free-text movement pattern a reviewer grouped an
// exercise under (e.g. "Squat", "Hip hinge", "Horizontal push",
// "Anti-rotation") — not a controlled enum, the sheet is the source of
// truth for which groups exist.
type MovementGroup string

// ReviewStatus says whether a sport scientist has signed off on an
// exercise. Approved is the ONLY status anything in production may
// select from — see Library.ApprovedExercises.
type ReviewStatus string

const (
	ReviewApproved ReviewStatus = "approved"
	ReviewPending  ReviewStatus = "pending"
)

// DemandLevel is a Low/Moderate/High rating, copied verbatim from the
// reviewer workbook's "Technical demand"/"Balance demand" columns.
type DemandLevel string

const (
	DemandLow      DemandLevel = "Low"
	DemandModerate DemandLevel = "Moderate"
	DemandHigh     DemandLevel = "High"
)

// Exercise is one entry in the Strength movement catalog — identity and
// classification only, never a prescription. The same exercise name can
// legitimately exist in two Zones (e.g. "Barbell Bench Press" in Upper
// and Full) — those are two distinct Exercise values with two distinct
// IDs, not deduplicated against each other.
type Exercise struct {
	// ID is a stable slug built from Zone + a normalized form of Name
	// (e.g. "lower_goblet_squat") — see exerciseID. Stable across
	// re-imports as long as Zone/Name don't change, so it's safe to
	// reference from outside this package (e.g. a future mesocycle pin).
	ID string `json:"id"`

	// ActivityType is always "strength" for everything this importer
	// produces today. Kept as a real field (not a hardcoded constant
	// elsewhere) so a future activity type's movement catalog can reuse
	// this exact type instead of inventing a near-duplicate.
	ActivityType activity.ID `json:"activity_type"`

	// Name is exactly what the reviewer workbook's "Exercise" column
	// says — never renamed or "cleaned up" by the importer or by hand.
	Name string `json:"name"`

	Zone          Zone          `json:"zone"`
	MovementGroup MovementGroup `json:"movement_group"`

	// ReviewStatus is Approved only when the workbook's "Reviewed by
	// Lina" cell is exactly "Yes" — every other value, including blank,
	// imports as Pending. Nothing downstream may select a Pending
	// exercise in production; see Library.ApprovedExercises and the
	// IncludePendingExercises config flag's dev-only escape hatch
	// (Library.ExercisesForZone).
	ReviewStatus ReviewStatus `json:"review_status"`

	// Equipment is the factual (not a judgment call) set of controlled
	// tags this exercise requires, mapped from the workbook's free-text
	// "Equipment required" column by the importer's equipment_mapping.json.
	// Unlike TechnicalDemand/BalanceDemand, this can be trusted before a
	// reviewer signs off — it's just what the exercise physically needs.
	Equipment []string `json:"equipment"`

	// TechnicalDemand and BalanceDemand are judgment calls nobody with
	// sport-science training has validated yet — copied from the sheet,
	// but NO logic may rely on these ratings in production until
	// DemandReviewed is true. They exist today purely so the data isn't
	// thrown away while the review backlog clears.
	TechnicalDemand DemandLevel `json:"technical_demand,omitempty"`
	BalanceDemand   DemandLevel `json:"balance_demand,omitempty"`

	// DemandReviewed is true only when ReviewStatus is Approved — it is
	// NOT a separate review pass, just a derived flag so a caller never
	// has to remember "demand ratings are only trustworthy on approved
	// rows" as an unwritten rule.
	DemandReviewed bool `json:"demand_reviewed"`

	// Flag and Note are informational-only fields from the "why is this
	// row pending" columns some workbook versions carry (e.g. "Lina
	// removed this", "Near something Lina removed"). They never change
	// ReviewStatus — they just explain it. Empty when the source
	// workbook layout has no Flag/Note columns.
	Flag string `json:"flag,omitempty"`
	Note string `json:"note,omitempty"`
}
