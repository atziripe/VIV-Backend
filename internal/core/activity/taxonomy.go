// Package activity models VIV's activity taxonomy — the first building
// block of the training-algorithm refactor described in "VIV Training
// Algorithm — Design Overview" (Section 3). It is pure reference data:
// no conflict-resolution cascade, goal profiles, or content-library
// selection lives here yet — see the design doc's Sections 4, 7-13 for
// what those are and why they're deliberately out of scope for this file.
//
// FIRST-DRAFT DATA: every TrainingEffect and RecoveryCost value below
// comes from an internal literature review, not yet reviewed or signed
// off by Lina or Juli — see "Training Effect & Recovery Cost — First
// Draft" and the TBD_Stefy_Lina.md buffer doc. Every seeded value is
// marked inline with:
//
//	first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
//
// so nothing downstream mistakes this for clinically final data.
//
// OPEN SCHEMA QUESTION: the six canonical TrainingEffect values (below)
// don't cleanly cover the muscular-endurance component of Pilates'
// higher-resistance work or Yoga's power/vinyasa styles. Rather than
// inventing a 7th value ("muscular_endurance") to force a fit, those
// entries map to the closest existing value (mobility) and are flagged
// inline at the entry — expanding the enum is Lina's call to confirm,
// not an implementation detail to decide silently.
package activity

// IntensityLevel is how hard a session is, independent of activity type.
// AR (active recovery / restorative) is deliberately its own level below
// L, not merged into it — Yoga and Mobility both need it as a distinct
// floor (design doc §3.1).
type IntensityLevel string

const (
	IntensityAR IntensityLevel = "AR"
	IntensityL  IntensityLevel = "L"
	IntensityM  IntensityLevel = "M"
	IntensityH  IntensityLevel = "H"
)

// ImpactLevel is how much joint/skeletal loading a session carries
// (design doc §3.3).
type ImpactLevel string

const (
	ImpactL ImpactLevel = "L"
	ImpactM ImpactLevel = "M"
	ImpactH ImpactLevel = "H"
)

// TrainingEffect is what a session actually trains — the six canonical
// values from the design doc's §3.4. See the OPEN SCHEMA QUESTION above
// before adding a 7th.
type TrainingEffect string

const (
	EffectStrength          TrainingEffect = "strength"
	EffectHypertrophy       TrainingEffect = "hypertrophy"
	EffectAerobicBase       TrainingEffect = "aerobic_base"
	EffectAnaerobicCapacity TrainingEffect = "anaerobic_capacity"
	EffectMobility          TrainingEffect = "mobility"
	EffectRecovery          TrainingEffect = "recovery"
)

// MuscleGroupMode says whether an activity's muscle-group focus is a
// single fixed value, or something the user selects (design doc §3.2).
type MuscleGroupMode string

const (
	MuscleGroupFixed          MuscleGroupMode = "fixed"
	MuscleGroupUserSelectable MuscleGroupMode = "user_selectable"
)

// MuscleGroup is the focus a session targets — either the fixed value for
// a Fixed-mode activity, or the default shown before a UserSelectable one
// has an actual selection.
//
// This intentionally only models the design doc's "primary classification"
// column (§3.2), not the secondary/incidental load column (e.g. Barre's
// "Full body" secondary, Running's "Core" secondary) — that richer model
// is out of scope for the taxonomy table itself.
type MuscleGroup string

const (
	MuscleGroupLower    MuscleGroup = "lower"
	MuscleGroupUpper    MuscleGroup = "upper"
	MuscleGroupFullBody MuscleGroup = "full_body"
	MuscleGroupCore     MuscleGroup = "core"
)

// ID identifies one of VIV's activity types.
type ID string

const (
	Strength         ID = "strength"
	Pilates          ID = "pilates"
	Barre            ID = "barre"
	Running          ID = "running"
	Cycling          ID = "cycling"
	HIIT             ID = "hiit"
	Functional       ID = "functional"
	Yoga             ID = "yoga"
	Mobility         ID = "mobility"
	Swimming         ID = "swimming"
	TeamRacketSports ID = "team_racket_sports"
)

// Type is the full taxonomy entry for one activity type — see the design
// doc's §3 for what each dimension means and why the model needs it.
type Type struct {
	ID ID

	// IntensityRange lists every intensity this activity can be assigned.
	// A single-element range (e.g. HIIT: [H]) is a genuinely fixed value,
	// not an arbitrary pick from a wider range — the conflict-resolution
	// cascade (built later, design doc §8) needs to detect "nothing to
	// relax here" when a lever tries to lower intensity or impact, hence
	// TestFixedRangesExposeExactlyOneValue below.
	IntensityRange []IntensityLevel

	// ImpactRange — same fixed-vs-ranged pattern as IntensityRange.
	ImpactRange []ImpactLevel

	MuscleGroupMode    MuscleGroupMode
	DefaultMuscleGroup MuscleGroup

	// TrainingEffect maps each intensity level in IntensityRange to the
	// set of effects a session at that intensity produces. A map — not a
	// flat field — because two things are true per the first-draft
	// literature review: (a) some types genuinely change effect with
	// intensity (Strength: L→mobility, M→hypertrophy, H→strength), and
	// (b) some types produce several effects concurrently at a given
	// intensity (HIIT: aerobic + anaerobic together, always). A flat
	// single value can't represent either case.
	TrainingEffect map[IntensityLevel][]TrainingEffect

	// RecoveryCost maps each intensity level in IntensityRange to a 0-3
	// cost — same per-intensity reasoning as TrainingEffect: recovery
	// cost scales with intensity within a type, it isn't one flat number
	// per activity.
	RecoveryCost map[IntensityLevel]int
}

// firstDraftMarker is the literal string every seeded TrainingEffect/
// RecoveryCost line below carries as a trailing comment. Kept as a named
// constant purely so TestEverySeededValueIsMarkedFirstDraft greps the
// source file for the exact same string this file's comments use.
const firstDraftMarker = "first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md"

// Types is the seeded activity taxonomy table — first-draft data, see the
// package doc comment above.
var Types = []Type{
	{
		ID:             Strength,
		IntensityRange: []IntensityLevel{IntensityL, IntensityM, IntensityH},
		// Base range treated as L-M per design doc §3.3: plyometric
		// strength can push to H in edge cases, but that's not the
		// activity's normal range.
		ImpactRange:     []ImpactLevel{ImpactL, ImpactM},
		MuscleGroupMode: MuscleGroupUserSelectable,
		// The design doc doesn't state a default for the pre-selection
		// state (§3.2 just says "user selects") — full_body picked here
		// as the most neutral placeholder, not sourced from the doc.
		DefaultMuscleGroup: MuscleGroupFullBody,
		TrainingEffect: map[IntensityLevel][]TrainingEffect{
			IntensityL: {EffectMobility},    // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM: {EffectHypertrophy}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: {EffectStrength},    // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
		RecoveryCost: map[IntensityLevel]int{
			IntensityL: 1, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM: 2, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: 3, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
	},
	{
		ID:              Pilates,
		IntensityRange:  []IntensityLevel{IntensityL, IntensityM},
		ImpactRange:     []ImpactLevel{ImpactL}, // fixed — "usually no jumping or landing" (design doc §3.3)
		MuscleGroupMode: MuscleGroupFixed,
		// Doc says "Full body or Core" (§3.2) — full_body picked as the
		// single representative value; Core is a real alternate this
		// single-value model can't carry.
		DefaultMuscleGroup: MuscleGroupFullBody,
		TrainingEffect: map[IntensityLevel][]TrainingEffect{
			IntensityL: {EffectMobility}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			// OPEN SCHEMA QUESTION (see package doc): reformer/high-
			// resistance Pilates trends toward core muscular endurance,
			// which has no clean home among the six canonical effects.
			// "mobility" is the closest fit, not a confirmed answer.
			IntensityM: {EffectMobility}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
		RecoveryCost: map[IntensityLevel]int{
			IntensityL: 0, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM: 1, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
	},
	{
		ID:              Barre,
		IntensityRange:  []IntensityLevel{IntensityL, IntensityM},
		ImpactRange:     []ImpactLevel{ImpactL, ImpactM},
		MuscleGroupMode: MuscleGroupFixed,
		// Doc says "Lower body + Core" primary, "Full body" secondary
		// (§3.2) — lower_body picked as the dominant single value.
		DefaultMuscleGroup: MuscleGroupLower,
		// WEAKEST-EVIDENCED ENTRY IN THE SET: inferred by analogy to
		// Pilates, not directly sourced from the literature review.
		TrainingEffect: map[IntensityLevel][]TrainingEffect{
			IntensityL: {EffectMobility}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			// Same open schema question as Pilates M — muscular
			// endurance component not cleanly representable.
			IntensityM: {EffectMobility}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
		RecoveryCost: map[IntensityLevel]int{
			// Source range was 0-1; storing the higher (more
			// conservative) bound since the field is a single int.
			IntensityL: 1, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			// Source range was 1-2; storing the higher bound.
			IntensityM: 2, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
	},
	{
		ID:                 Running,
		IntensityRange:     []IntensityLevel{IntensityL, IntensityM, IntensityH},
		ImpactRange:        []ImpactLevel{ImpactH}, // FIXED — "easy running is still repetitive impact" (design doc §3.3), no lower option
		MuscleGroupMode:    MuscleGroupFixed,
		DefaultMuscleGroup: MuscleGroupLower,
		TrainingEffect: map[IntensityLevel][]TrainingEffect{
			IntensityL: {EffectAerobicBase}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			// "Transitional" per the literature review — tempo work sits
			// between base aerobic and anaerobic capacity; aerobic_base
			// is the closer of the two.
			IntensityM: {EffectAerobicBase},       // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: {EffectAnaerobicCapacity}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
		RecoveryCost: map[IntensityLevel]int{
			IntensityL: 1, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM: 2, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: 3, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
	},
	{
		ID:                 Cycling,
		IntensityRange:     []IntensityLevel{IntensityL, IntensityM, IntensityH},
		ImpactRange:        []ImpactLevel{ImpactL}, // FIXED — "minimal impact, even when intensity is high" (design doc §3.3)
		MuscleGroupMode:    MuscleGroupFixed,
		DefaultMuscleGroup: MuscleGroupLower,
		TrainingEffect: map[IntensityLevel][]TrainingEffect{
			IntensityL: {EffectAerobicBase},       // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM: {EffectAerobicBase},       // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: {EffectAnaerobicCapacity}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
		RecoveryCost: map[IntensityLevel]int{
			// Source range was 0-1; storing the higher bound.
			IntensityL: 1, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM: 1, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: 2, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
	},
	{
		ID:             HIIT,
		IntensityRange: []IntensityLevel{IntensityH}, // FIXED — "true HIIT means repeated Z4-5 or near-maximal efforts" (design doc §3.1)
		ImpactRange:    []ImpactLevel{ImpactM, ImpactH},
		// UserSelectable per explicit product direction: "Strength/HIIT/
		// Functional are UserSelectable" — matches design doc §3.2
		// ("Full body by default; User can select Lower/Upper/Full").
		MuscleGroupMode:    MuscleGroupUserSelectable,
		DefaultMuscleGroup: MuscleGroupFullBody,
		TrainingEffect: map[IntensityLevel][]TrainingEffect{
			// first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: {EffectAnaerobicCapacity, EffectAerobicBase},
		},
		RecoveryCost: map[IntensityLevel]int{
			// OPEN MODELING GAP: HIIT's real recovery-cost variation is
			// driven by IMPACT, not intensity (fixed at H) — low-impact
			// (bike/rower) is 2, high-impact (running/jump-based) is 3
			// per the literature review. A map keyed by IntensityLevel
			// can't represent that second axis at all. Storing the
			// higher (high-impact, more conservative) value here rather
			// than silently averaging or picking the lower one — this
			// is a real gap for whoever builds the conflict-resolution
			// cascade to revisit, not a stopgap to treat as settled.
			IntensityH: 3, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
	},
	{
		ID:                 Functional,
		IntensityRange:     []IntensityLevel{IntensityL, IntensityM, IntensityH},
		ImpactRange:        []ImpactLevel{ImpactL, ImpactM, ImpactH},
		MuscleGroupMode:    MuscleGroupUserSelectable, // "Strength/HIIT/Functional are UserSelectable"
		DefaultMuscleGroup: MuscleGroupFullBody,
		TrainingEffect: map[IntensityLevel][]TrainingEffect{
			IntensityL: {EffectMobility}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			// Concurrent effects, not a single dominant one — flagging
			// that Functional (and activities like it) may need more
			// than a per-intensity map can express as this model grows
			// beyond the taxonomy table (e.g. a per-exercise-block view).
			IntensityM: {EffectStrength, EffectAerobicBase},                          // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: {EffectStrength, EffectAerobicBase, EffectAnaerobicCapacity}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
		RecoveryCost: map[IntensityLevel]int{
			IntensityL: 1, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM: 2, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: 3, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
	},
	{
		ID:              Yoga,
		IntensityRange:  []IntensityLevel{IntensityAR, IntensityL, IntensityM},
		ImpactRange:     []ImpactLevel{ImpactL}, // fixed — "power yoga can be intense but remains mostly low impact" (design doc §3.3)
		MuscleGroupMode: MuscleGroupFixed,
		// Doc says "Full body / Mobility" (§3.2) — a poor fit for a
		// muscle-group value in the first place (it's describing what
		// the session trains, not which muscles); full_body picked as
		// the least-wrong single choice.
		DefaultMuscleGroup: MuscleGroupFullBody,
		TrainingEffect: map[IntensityLevel][]TrainingEffect{
			IntensityAR: {EffectRecovery}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityL:  {EffectMobility}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			// Same open schema question as Pilates M: power/vinyasa's
			// secondary muscular-endurance component isn't cleanly
			// representable among the six canonical effects.
			IntensityM: {EffectMobility}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
		RecoveryCost: map[IntensityLevel]int{
			IntensityAR: 0, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityL:  0, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM:  0, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
	},
	{
		ID:              Mobility,
		IntensityRange:  []IntensityLevel{IntensityAR, IntensityL},
		ImpactRange:     []ImpactLevel{ImpactL}, // fixed — "recovery-oriented and non-impact" (design doc §3.3)
		MuscleGroupMode: MuscleGroupFixed,
		// Doc says "Mobility / Recovery" (§3.2) — this doesn't map onto a
		// muscle-group value at all (it describes intent, not target);
		// full_body picked as the least-wrong single choice, same open
		// question as Yoga above.
		DefaultMuscleGroup: MuscleGroupFullBody,
		TrainingEffect: map[IntensityLevel][]TrainingEffect{
			IntensityAR: {EffectRecovery, EffectMobility}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityL:  {EffectRecovery, EffectMobility}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
		RecoveryCost: map[IntensityLevel]int{
			IntensityAR: 0, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityL:  0, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
	},
	{
		ID:                 Swimming,
		IntensityRange:     []IntensityLevel{IntensityL, IntensityM, IntensityH},
		ImpactRange:        []ImpactLevel{ImpactL}, // fixed — "very low weight-bearing impact" (design doc §3.3)
		MuscleGroupMode:    MuscleGroupFixed,
		DefaultMuscleGroup: MuscleGroupFullBody,
		// WEAKEST-EVIDENCED ENTRY ALONGSIDE BARRE: not directly sourced
		// from the literature review, inferred from the Running/Cycling
		// low/medium/high pattern.
		TrainingEffect: map[IntensityLevel][]TrainingEffect{
			IntensityL: {EffectAerobicBase},       // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM: {EffectAerobicBase},       // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: {EffectAnaerobicCapacity}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
		RecoveryCost: map[IntensityLevel]int{
			IntensityL: 0, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM: 1, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: 2, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
	},
	{
		ID:             TeamRacketSports,
		IntensityRange: []IntensityLevel{IntensityL, IntensityM, IntensityH},
		// Union of Team sports (H, fixed) and Racket sports (M-H, ranged)
		// per the design doc's separate impact rows (§3.3) — Team's H is
		// already inside Racket's M-H range, so combining the two under
		// one taxonomy entry doesn't lose information.
		ImpactRange:        []ImpactLevel{ImpactM, ImpactH},
		MuscleGroupMode:    MuscleGroupFixed,
		DefaultMuscleGroup: MuscleGroupFullBody,
		TrainingEffect: map[IntensityLevel][]TrainingEffect{
			IntensityL: {EffectAerobicBase},                          // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM: {EffectAnaerobicCapacity, EffectAerobicBase}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityH: {EffectAnaerobicCapacity, EffectAerobicBase}, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
		RecoveryCost: map[IntensityLevel]int{
			IntensityL: 1, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			IntensityM: 2, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
			// STRONGEST-EVIDENCED RECOVERY-COST VALUE IN THE WHOLE SET —
			// converging match-play studies. Do not water this down to a
			// lower number even though 3 is the max of the scale.
			IntensityH: 3, // first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md
		},
	},
}

// ByID looks up one activity type by ID.
func ByID(id ID) (Type, bool) {
	for _, t := range Types {
		if t.ID == id {
			return t, true
		}
	}
	return Type{}, false
}
