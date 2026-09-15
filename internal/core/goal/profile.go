// Package goal models VIV's goal profiles — the second building block of
// the training-algorithm refactor described in "VIV Training Algorithm —
// Design Overview" (Sections 4 and 8), built on top of the activity
// taxonomy in internal/core/activity.
//
// Like that package, this is pure reference data: no conflict-resolution
// cascade lives here yet. GoalProfile.ConflictLeverOrder and
// CompositionPriorities are the sequences that cascade will walk later
// (VIV-105) — this package only defines what those sequences are per
// goal, not the logic that walks them.
package goal

import "viv/internal/core/activity"

// ConflictLever is one possible adjustment VIV can try when a planned
// session doesn't fit the day's readiness (design doc §8). Exactly six
// values — this is a closed set, not extensible per goal.
type ConflictLever string

const (
	ProtectType ConflictLever = "protect_type"
	// RelaxIntensity keeps the same activity at a lower effort level
	RelaxIntensity ConflictLever = "relax_intensity"
	// RelaxImpact keeps the same activity with less joint loading
	RelaxImpact ConflictLever = "relax_impact"
	// ReduceDuration keeps the same activity and intent
	ReduceDuration ConflictLever = "reduce_duration"
	// SimplifyComplexity keeps the same activity
	SimplifyComplexity ConflictLever = "simplify_complexity"
	// SubstituteType swaps to a different activity — last resort only
	SubstituteType ConflictLever = "substitute_type"
)

// PriorityTier says how much a goal leans on a given TrainingEffect when
// filling the week's session slots — an ORDERED CASCADE (Primary first,
// then Supporting, then Occasional/Optional only if budget remains),
// consumed later by VIV-104. Deliberately not a numeric weight: we don't
// have data to calibrate arbitrary percentages, and an invented number
// would look more authoritative than it is.
type PriorityTier string

const (
	PriorityPrimary    PriorityTier = "primary"
	PrioritySupporting PriorityTier = "supporting"
	PriorityOccasional PriorityTier = "occasional"
	PriorityOptional   PriorityTier = "optional"
)

// CompositionPriority pairs a TrainingEffect (reused from the activity
// taxonomy, VIV-101 — not a separate ActivityCategory classification, one
// less parallel taxonomy to keep in sync) with the tier a goal gives it.
type CompositionPriority struct {
	Effect activity.TrainingEffect
	Tier   PriorityTier
}

type ID string

const (
	StrengthMuscle       ID = "strength_muscle"
	EndurancePerformance ID = "endurance_performance"
	BodyComposition      ID = "body_composition"
	ConsistencyWellbeing ID = "consistency_wellbeing"
)

// ProtectedActivityUserPrimaryEndurance is a marker value, not a real
// activity.ID from the taxonomy table. Unlike StrengthMuscle's protected
// type (always "strength"), EndurancePerformance's protected type isn't
// fixed — it's "whichever of Running/Cycling/Swimming is the user's
// primary endurance activity in her catalog" (per product direction for
// this field). Resolving this marker into an actual activity.ID from a
// specific user's catalog is cascade logic (design doc §8, VIV-105) that
// doesn't exist yet — this package only records that the resolution is
// per-user, not per-goal.
const ProtectedActivityUserPrimaryEndurance activity.ID = "user_primary_endurance"

// GoalProfile is the full config entry for one goal — see the design
// doc's §4 (what each goal protects/prioritizes) and §8.1 (the resulting
// adjustment order) for the reasoning behind each field below.
type GoalProfile struct {
	ID ID

	// ProtectedActivityType is the activity type this goal protects via
	// the ProtectType lever — nil means this goal has no protected type
	// at all. Body composition and Consistency & wellbeing don't protect
	// a single session type by design (design doc §8.1: "Body
	// composition and Consistency & wellbeing don't protect a single
	// session type by design; their priority is balance and flexibility
	// respectively") — ConflictLeverOrder omits ProtectType entirely for
	// those two, not just sets this to nil.
	ProtectedActivityType *activity.ID

	// ConflictLeverOrder is the sequence tried when a planned session
	// doesn't fit the day's readiness, stopping at the first lever that
	// resolves the mismatch (design doc §8). SubstituteType is always
	// last across every goal — see TestSubstituteTypeIsAlwaysLast.
	ConflictLeverOrder []ConflictLever

	// CompositionPriorities is a direct translation of the existing
	// goal-priority table in design doc §4 (specifically the "Training
	// priority" and "Cardio role" rows) into an ordered cascade — not new
	// invented data, but still first-draft/not yet implemented in code,
	// so kept easy to revise. Every goal has at least one "primary"
	// entry — see TestEveryGoalHasAtLeastOnePrimaryPriority.
	CompositionPriorities []CompositionPriority

	// CopyTone is a stable identifier the weekly-note generator (design
	// doc §15, VIV-113) looks up later against a copy/localization
	// system — not the copy text itself.
	CopyTone string
}

func idPtr(id activity.ID) *activity.ID { return &id }

// Profiles is the seeded goal-profile table.
var Profiles = []GoalProfile{
	{
		ID:                    StrengthMuscle,
		ProtectedActivityType: idPtr(activity.Strength),
		ConflictLeverOrder: []ConflictLever{
			ProtectType, RelaxIntensity, RelaxImpact, ReduceDuration, SimplifyComplexity, SubstituteType,
		},
		CompositionPriorities: []CompositionPriority{
			{Effect: activity.EffectStrength, Tier: PriorityPrimary},
			{Effect: activity.EffectHypertrophy, Tier: PriorityPrimary},
			{Effect: activity.EffectAerobicBase, Tier: PrioritySupporting},
			{Effect: activity.EffectMobility, Tier: PrioritySupporting},
			{Effect: activity.EffectAnaerobicCapacity, Tier: PriorityOccasional},
		},
		CopyTone: "strength_muscle_tone",
	},
	{
		ID:                    EndurancePerformance,
		ProtectedActivityType: idPtr(ProtectedActivityUserPrimaryEndurance),
		ConflictLeverOrder: []ConflictLever{
			ProtectType, RelaxIntensity, RelaxImpact, ReduceDuration, SimplifyComplexity, SubstituteType,
		},
		CompositionPriorities: []CompositionPriority{
			{Effect: activity.EffectAerobicBase, Tier: PriorityPrimary},
			{Effect: activity.EffectAnaerobicCapacity, Tier: PriorityPrimary},
			{Effect: activity.EffectStrength, Tier: PrioritySupporting},
			{Effect: activity.EffectMobility, Tier: PrioritySupporting},
		},
		CopyTone: "endurance_performance_tone",
	},
	{
		ID:                    BodyComposition,
		ProtectedActivityType: nil, // no protected type, by design — see the field's doc comment
		ConflictLeverOrder: []ConflictLever{
			RelaxImpact, RelaxIntensity, ReduceDuration, SimplifyComplexity, SubstituteType,
		},
		CompositionPriorities: []CompositionPriority{
			{Effect: activity.EffectStrength, Tier: PriorityPrimary},
			{Effect: activity.EffectHypertrophy, Tier: PriorityPrimary},
			{Effect: activity.EffectAerobicBase, Tier: PrioritySupporting},
			{Effect: activity.EffectMobility, Tier: PrioritySupporting},
			{Effect: activity.EffectAnaerobicCapacity, Tier: PriorityOccasional},
		},
		CopyTone: "body_composition_tone",
	},
	{
		ID:                    ConsistencyWellbeing,
		ProtectedActivityType: nil, // no protected type, by design — see the field's doc comment
		ConflictLeverOrder: []ConflictLever{
			ReduceDuration, SimplifyComplexity, RelaxIntensity, RelaxImpact, SubstituteType,
		},
		// Deliberately short — per design doc §4 this goal has no fixed
		// hierarchy beyond protecting continuity; everything else is
		// governed more by the user's own catalog/preference than by
		// the goal.
		CompositionPriorities: []CompositionPriority{
			{Effect: activity.EffectMobility, Tier: PriorityPrimary},
			{Effect: activity.EffectRecovery, Tier: PriorityPrimary},
		},
		CopyTone: "consistency_wellbeing_tone",
	},
}

// ByID looks up one goal profile by ID.
func ByID(id ID) (GoalProfile, bool) {
	for _, p := range Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return GoalProfile{}, false
}
