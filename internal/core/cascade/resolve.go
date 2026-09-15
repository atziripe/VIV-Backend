// Package cascade implements the conflict-resolution cascade described in
// "VIV Training Algorithm — Design Overview" §8: what happens when a
// planned session doesn't fit a user's actual state. Depends on VIV-101
// (internal/core/activity), VIV-102 (internal/core/goal), and VIV-104
// (internal/core/weeklytarget) — explored all three before writing this.
//
// This is the most heavily tested package in the refactor on purpose: it
// is the one piece that actually decides what happens to a real user's
// session on a real day.
package cascade

import (
	"fmt"

	"viv/internal/core/activity"
	"viv/internal/core/goal"
	weeklytarget "viv/internal/core/weekly_target"
)

// ============================================================================
// Inputs
// ============================================================================

// Activity is one candidate session: an activity type at a specific
// intensity/impact/muscle-group. It's the shape both the "natural/default
// candidate from the user's catalog" (ResolveSlotConflict's input) and the
// final resolved assignment (SlotAssignment) share the core of.
type Activity struct {
	ActivityType activity.ID
	Intensity    activity.IntensityLevel
	Impact       activity.ImpactLevel
	MuscleGroup  activity.MuscleGroup
}

// UserCatalog is "which of VIV's supported activity types she actually
// practices" (design doc §2). Modeled here, in the cascade package, since
// this is the first piece that actually needs it — if a broader user-
// profile package emerges later (VIV-106+) this can move there.
type UserCatalog struct {
	Activities []activity.ID
	// PrimaryEndurance is the user's preferred endurance activity among
	// Running/Cycling/Swimming, if she practices more than one. nil means
	// "let the catalog decide" — ResolveProtectedType then picks the
	// first of Running/Cycling/Swimming actually present in Activities.
	PrimaryEndurance *activity.ID
}

// Has reports whether id is in the catalog.
func (c UserCatalog) Has(id activity.ID) bool {
	for _, a := range c.Activities {
		if a == id {
			return true
		}
	}
	return false
}

// ResolveProtectedType turns a GoalProfile's ProtectedActivityType
// (VIV-102) into a concrete activity.ID for THIS user. For every goal
// except endurance_performance, that's just the value itself. For
// endurance_performance, ProtectedActivityType is the
// goal.ProtectedActivityUserPrimaryEndurance marker — VIV-102 explicitly
// left resolving it as cascade logic, which is what this is. Returns
// ok=false when there's no protected type at all (nil) or the marker
// can't be resolved (no endurance activity in the catalog).
func (c UserCatalog) ResolveProtectedType(protected *activity.ID) (activity.ID, bool) {
	if protected == nil {
		return "", false
	}
	if *protected != goal.ProtectedActivityUserPrimaryEndurance {
		return *protected, true
	}
	if c.PrimaryEndurance != nil && c.Has(*c.PrimaryEndurance) {
		return *c.PrimaryEndurance, true
	}
	for _, candidate := range []activity.ID{activity.Running, activity.Cycling, activity.Swimming} {
		if c.Has(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// ============================================================================
// Output
// ============================================================================

// AdjustmentLever mirrors goal.ConflictLever's values but is its own type:
// it needs one state goal.ConflictLever deliberately does NOT have —
// AdjustmentForcedRelax, the substitution-cap fallback — and
// goal.ConflictLever is documented as a closed 6-value set (VIV-102). Zero
// value (AdjustmentNone) means "candidate already satisfied the target,
// no lever was applied at all."
type AdjustmentLever string

const (
	AdjustmentNone               AdjustmentLever = ""
	AdjustmentProtectType        AdjustmentLever = "protect_type"
	AdjustmentRelaxIntensity     AdjustmentLever = "relax_intensity"
	AdjustmentRelaxImpact        AdjustmentLever = "relax_impact"
	AdjustmentReduceDuration     AdjustmentLever = "reduce_duration"
	AdjustmentSimplifyComplexity AdjustmentLever = "simplify_complexity"
	AdjustmentSubstituteType     AdjustmentLever = "substitute_type"
	// AdjustmentForcedRelax marks the substitution-cap fallback: 3
	// substitutions were already used this week, so instead of a 4th,
	// RelaxImpact then RelaxIntensity were force-applied to the original
	// candidate regardless of the goal's own order or what was already
	// tried earlier in this call.
	AdjustmentForcedRelax AdjustmentLever = "forced_relax"
)

func fromConflictLever(l goal.ConflictLever) AdjustmentLever { return AdjustmentLever(l) }

// LoadTier — set by ProtectType only; every other lever leaves it Standard.
type LoadTier string

const (
	LoadStandard LoadTier = "standard"
	LoadReduced  LoadTier = "reduced"
)

// DurationTier — set by ReduceDuration. Always Standard until
// DefaultContentLibrary is wired to a real implementation that reports a
// short variant exists for the given activity type (see its doc comment).
type DurationTier string

const (
	DurationStandard DurationTier = "standard"
	DurationShort    DurationTier = "short"
)

// ComplexityTier — set by SimplifyComplexity. Always Standard today, same
// reason as DurationTier.
type ComplexityTier string

const (
	ComplexityStandard   ComplexityTier = "standard"
	ComplexitySimplified ComplexityTier = "simplified"
)

// SlotAssignment is the cascade's output for one session slot.
type SlotAssignment struct {
	ActivityType activity.ID
	Intensity    activity.IntensityLevel
	Impact       activity.ImpactLevel
	MuscleGroup  activity.MuscleGroup

	Substituted      bool
	OriginalType     activity.ID
	SubstituteReason string

	DurationTier   DurationTier
	ComplexityTier ComplexityTier
	LoadTier       LoadTier

	AdjustmentLever AdjustmentLever

	// UserOverrode isn't populated by this function — VIV-107 populates
	// it later, once a user has had a chance to react to an automatic
	// adjustment (design doc §9). It exists on the struct now so
	// downstream code has a stable shape to build against.
	UserOverrode bool
}

// ============================================================================
// Content library stub (design doc §13 — pending VIV-112)
// ============================================================================

// Variant is a pre-built alternate version of a session.
type Variant string

const (
	VariantShort      Variant = "short"
	VariantSimplified Variant = "simplified"
)

// ContentLibrary is what the cascade needs from the content library to
// know whether a pre-built short/simplified variant exists for a given
// activity. Modeled as an interface so a real implementation can be
// swapped in later without touching this package.
type ContentLibrary interface {
	HasVariant(id activity.ID, v Variant) bool
}

// stubContentLibrary always reports no variants exist. It's the initial
// value of DefaultContentLibrary below, before anything real is wired in.
type stubContentLibrary struct{}

func (stubContentLibrary) HasVariant(activity.ID, Variant) bool { return false }

// DefaultContentLibrary is what ResolveSlotConflict checks for
// ReduceDuration/SimplifyComplexity applicability. ResolveSlotConflict's
// signature is fixed by the task spec (no ContentLibrary parameter), so
// this is a package-level swappable default rather than an injected
// dependency — set it once at boot (e.g. `cascade.DefaultContentLibrary =
// realLibrary`) the same way sql.Register-style package globals work.
//
// Starts as stubContentLibrary — always false — because the actual
// library/variant data didn't exist yet when VIV-105 was built (the
// Lina-led content curation work, design doc §13). VIV-112
// (internal/core/content.SessionLibrary) is the real implementation:
// it's what this var should be assigned once real short/simplified
// session variants exist for a given activity type. Until then, this
// correctly keeps ReduceDuration/SimplifyComplexity reporting "not
// applicable" — never silently claiming a variant exists when it doesn't.
var DefaultContentLibrary ContentLibrary = stubContentLibrary{}

// ============================================================================
// ResolveSlotConflict
// ============================================================================

// ResolveSlotConflict decides what actually happens to one planned
// session, given the week's target and the user's real state — design doc
// §8. If candidate already satisfies target, it's returned unchanged with
// AdjustmentLever == AdjustmentNone and the cascade never runs at all.
// Otherwise goal.ConflictLeverOrder is walked in order, applying and
// re-checking each applicable lever, stopping at the first one that
// resolves the mismatch.
func ResolveSlotConflict(
	target weeklytarget.WeeklyTarget,
	candidate Activity,
	catalog UserCatalog,
	g goal.GoalProfile,
	substitutionsUsedThisWeek int,
) (SlotAssignment, error) {
	if g.ID == "" {
		return SlotAssignment{}, fmt.Errorf("cascade: empty GoalProfile")
	}

	candType, ok := activity.ByID(candidate.ActivityType)
	if !ok {
		return SlotAssignment{}, fmt.Errorf("cascade: unknown activity type %q", candidate.ActivityType)
	}
	if !intensityInRange(candType.IntensityRange, candidate.Intensity) {
		return SlotAssignment{}, fmt.Errorf("cascade: intensity %q is not valid for %q", candidate.Intensity, candidate.ActivityType)
	}
	if !impactInRange(candType.ImpactRange, candidate.Impact) {
		return SlotAssignment{}, fmt.Errorf("cascade: impact %q is not valid for %q", candidate.Impact, candidate.ActivityType)
	}

	if satisfiesTarget(candidate, candType, target) {
		return baseAssignment(candidate, candidate.ActivityType, AdjustmentNone, LoadStandard), nil
	}

	protectedType, hasProtected := catalog.ResolveProtectedType(g.ProtectedActivityType)

	working := candidate
	loadReduced := false

	for _, lever := range g.ConflictLeverOrder {
		switch lever {

		case goal.ProtectType:
			if !hasProtected || working.ActivityType != protectedType {
				continue
			}
			if !relaxIntensityApplicable(candType, working.Intensity) {
				continue
			}
			working.Intensity = relaxIntensityToFit(candType, working.Intensity, target.IntensityCap)
			loadReduced = true
			if satisfiesTarget(working, candType, target) {
				return baseAssignment(working, candidate.ActivityType, AdjustmentProtectType, LoadReduced), nil
			}

		case goal.RelaxIntensity:
			if !relaxIntensityApplicable(candType, working.Intensity) {
				continue
			}
			working.Intensity = relaxIntensityToFit(candType, working.Intensity, target.IntensityCap)
			if satisfiesTarget(working, candType, target) {
				loadTier := LoadStandard
				if loadReduced {
					loadTier = LoadReduced
				}
				return baseAssignment(working, candidate.ActivityType, fromConflictLever(lever), loadTier), nil
			}

		case goal.RelaxImpact:
			if !relaxImpactApplicable(candType, working.Impact) {
				continue
			}
			working.Impact = relaxImpactToFit(candType, working.Impact, target.ImpactCap)
			if satisfiesTarget(working, candType, target) {
				loadTier := LoadStandard
				if loadReduced {
					loadTier = LoadReduced
				}
				return baseAssignment(working, candidate.ActivityType, fromConflictLever(lever), loadTier), nil
			}

		case goal.ReduceDuration:
			if !DefaultContentLibrary.HasVariant(candidate.ActivityType, VariantShort) {
				continue // always true today — see the ContentLibrary stub's TODO
			}
			// Unreachable until VIV-112 provides real variant data, but
			// kept so the shape is right once it isn't stubbed:
			assignment := baseAssignment(working, candidate.ActivityType, AdjustmentReduceDuration, loadTierOf(loadReduced))
			assignment.DurationTier = DurationShort
			if satisfiesTarget(working, candType, target) {
				return assignment, nil
			}

		case goal.SimplifyComplexity:
			if !DefaultContentLibrary.HasVariant(candidate.ActivityType, VariantSimplified) {
				continue // always true today — see the ContentLibrary stub's TODO
			}
			assignment := baseAssignment(working, candidate.ActivityType, AdjustmentSimplifyComplexity, loadTierOf(loadReduced))
			assignment.ComplexityTier = ComplexitySimplified
			if satisfiesTarget(working, candType, target) {
				return assignment, nil
			}

		case goal.SubstituteType:
			if substitutionsUsedThisWeek >= 3 {
				return forcedRelax(candidate, candType, target), nil
			}
			return substitute(candidate, catalog, target)
		}
	}

	// Unreachable in practice: SubstituteType is always last in every
	// goal's ConflictLeverOrder (VIV-102) and always either resolves
	// (catalog or out-of-catalog) or returns its own error above — this
	// is only reached if a goal's order were missing SubstituteType
	// entirely, which VIV-102's own tests already guard against.
	return SlotAssignment{}, fmt.Errorf("cascade: exhausted %s's conflict lever order without resolving the conflict", g.ID)
}

func loadTierOf(reduced bool) LoadTier {
	if reduced {
		return LoadReduced
	}
	return LoadStandard
}

// baseAssignment builds a SlotAssignment from a resolved Activity.
// originalType is passed explicitly (rather than closed over) so both
// ResolveSlotConflict and SuggestSafetyRelax (VIV-107) can share it.
func baseAssignment(a Activity, originalType activity.ID, lever AdjustmentLever, loadTier LoadTier) SlotAssignment {
	return SlotAssignment{
		ActivityType:    a.ActivityType,
		Intensity:       a.Intensity,
		Impact:          a.Impact,
		MuscleGroup:     a.MuscleGroup,
		OriginalType:    originalType,
		LoadTier:        loadTier,
		DurationTier:    DurationStandard,
		ComplexityTier:  ComplexityStandard,
		AdjustmentLever: lever,
	}
}

// ============================================================================
// VIV-107 support: checking fit without running the cascade, and a
// safety-only variant of the cascade for user-overridden slots.
// ============================================================================

// SatisfiesTarget reports whether a candidate activity already satisfies a
// WeeklyTarget's caps and composition priorities — the same check
// ResolveSlotConflict uses internally to decide whether it even needs to
// run. Exported for daily adaptation (VIV-107), which needs to ask this
// question about an already-scheduled session WITHOUT risking running the
// cascade over it — if nothing actually changed, the caller wants to
// return the existing SlotAssignment completely untouched (substitution
// history, UserOverrode, etc. intact), not a freshly rebuilt one.
func SatisfiesTarget(a Activity, target weeklytarget.WeeklyTarget) (bool, error) {
	t, ok := activity.ByID(a.ActivityType)
	if !ok {
		return false, fmt.Errorf("cascade: unknown activity type %q", a.ActivityType)
	}
	if !intensityInRange(t.IntensityRange, a.Intensity) {
		return false, fmt.Errorf("cascade: intensity %q is not valid for %q", a.Intensity, a.ActivityType)
	}
	if !impactInRange(t.ImpactRange, a.Impact) {
		return false, fmt.Errorf("cascade: impact %q is not valid for %q", a.Impact, a.ActivityType)
	}
	return satisfiesTarget(a, t, target), nil
}

// SuggestSafetyRelax is the constrained cascade VIV-107 runs against a
// slot the user has manually overridden (SlotAssignment.UserOverrode):
// design doc §9 says automatic adjustments explain, they never force, and
// draws the line at "safety-relevant" — RelaxIntensity and RelaxImpact
// only. Every other lever (ProtectType included — it's a stimulus
// decision, not a safety one) is skipped entirely here, regardless of the
// goal's own ConflictLeverOrder, and this function never mutates
// anything — it only ever returns a candidate result for the caller to
// offer as a suggestion the user can accept or dismiss.
//
// The two levers are still tried in the goal's own relative order between
// them, and a partial adjustment from one carries into the other — same
// cumulative behavior as the real cascade, just restricted to a subset of
// levers.
//
// ok=false means neither lever can bring the candidate under the target's
// caps — the caller should surface "no safe adjustment available", never
// fall through to a lever this function deliberately excludes.
func SuggestSafetyRelax(
	target weeklytarget.WeeklyTarget,
	candidate Activity,
	g goal.GoalProfile,
) (SlotAssignment, bool, error) {
	if g.ID == "" {
		return SlotAssignment{}, false, fmt.Errorf("cascade: empty GoalProfile")
	}

	candType, ok := activity.ByID(candidate.ActivityType)
	if !ok {
		return SlotAssignment{}, false, fmt.Errorf("cascade: unknown activity type %q", candidate.ActivityType)
	}
	if !intensityInRange(candType.IntensityRange, candidate.Intensity) {
		return SlotAssignment{}, false, fmt.Errorf("cascade: intensity %q is not valid for %q", candidate.Intensity, candidate.ActivityType)
	}
	if !impactInRange(candType.ImpactRange, candidate.Impact) {
		return SlotAssignment{}, false, fmt.Errorf("cascade: impact %q is not valid for %q", candidate.Impact, candidate.ActivityType)
	}

	working := candidate

	for _, lever := range g.ConflictLeverOrder {
		switch lever {
		case goal.RelaxIntensity:
			if !relaxIntensityApplicable(candType, working.Intensity) {
				continue
			}
			working.Intensity = relaxIntensityToFit(candType, working.Intensity, target.IntensityCap)
			if satisfiesTarget(working, candType, target) {
				return baseAssignment(working, candidate.ActivityType, fromConflictLever(lever), LoadStandard), true, nil
			}

		case goal.RelaxImpact:
			if !relaxImpactApplicable(candType, working.Impact) {
				continue
			}
			working.Impact = relaxImpactToFit(candType, working.Impact, target.ImpactCap)
			if satisfiesTarget(working, candType, target) {
				return baseAssignment(working, candidate.ActivityType, fromConflictLever(lever), LoadStandard), true, nil
			}

		default:
			// Every other lever — ProtectType, ReduceDuration,
			// SimplifyComplexity, SubstituteType — is deliberately not
			// handled here, on purpose: it must never fire over a user
			// override, no matter where it sits in the goal's order.
			continue
		}
	}

	return SlotAssignment{}, false, nil
}

// ============================================================================
// Satisfaction check
// ============================================================================

func satisfiesTarget(a Activity, t activity.Type, target weeklytarget.WeeklyTarget) bool {
	if intensityRank[a.Intensity] > intensityRank[target.IntensityCap] {
		return false
	}
	if impactRank[a.Impact] > impactRank[target.ImpactCap] {
		return false
	}
	return producesAPrioritizedEffect(a, t, target.CompositionPriorities)
}

func producesAPrioritizedEffect(a Activity, t activity.Type, priorities []goal.CompositionPriority) bool {
	effects := t.TrainingEffect[a.Intensity]
	for _, e := range effects {
		for _, p := range priorities {
			if p.Effect == e {
				return true
			}
		}
	}
	return false
}

// ============================================================================
// Intensity/impact ordering, applicability, and relax-to-fit
// ============================================================================

var intensityRank = map[activity.IntensityLevel]int{
	activity.IntensityAR: 0,
	activity.IntensityL:  1,
	activity.IntensityM:  2,
	activity.IntensityH:  3,
}

var impactRank = map[activity.ImpactLevel]int{
	activity.ImpactL: 0,
	activity.ImpactM: 1,
	activity.ImpactH: 2,
}

func intensityInRange(levels []activity.IntensityLevel, level activity.IntensityLevel) bool {
	for _, l := range levels {
		if l == level {
			return true
		}
	}
	return false
}

func impactInRange(levels []activity.ImpactLevel, level activity.ImpactLevel) bool {
	for _, l := range levels {
		if l == level {
			return true
		}
	}
	return false
}

func lowestIntensity(levels []activity.IntensityLevel) activity.IntensityLevel {
	lowest := levels[0]
	for _, l := range levels[1:] {
		if intensityRank[l] < intensityRank[lowest] {
			lowest = l
		}
	}
	return lowest
}

func lowestImpact(levels []activity.ImpactLevel) activity.ImpactLevel {
	lowest := levels[0]
	for _, l := range levels[1:] {
		if impactRank[l] < impactRank[lowest] {
			lowest = l
		}
	}
	return lowest
}

// relaxIntensityApplicable — design doc §8 / task spec, verbatim: only if
// the activity's IntensityRange has more than one value AND the candidate
// isn't already at the lowest available value.
func relaxIntensityApplicable(t activity.Type, current activity.IntensityLevel) bool {
	if len(t.IntensityRange) <= 1 {
		return false
	}
	return current != lowestIntensity(t.IntensityRange)
}

// relaxImpactApplicable — same pattern as relaxIntensityApplicable, using
// ImpactRange. Several activities have a FIXED impact range (Running=H
// only, Cycling/Pilates/Yoga/Mobility/Swimming=L only) — those correctly
// report false here rather than silently no-op-ing.
func relaxImpactApplicable(t activity.Type, current activity.ImpactLevel) bool {
	if len(t.ImpactRange) <= 1 {
		return false
	}
	return current != lowestImpact(t.ImpactRange)
}

// relaxIntensityToFit picks the highest value in the activity's own
// IntensityRange that still fits under cap — preserving as much stimulus
// as possible while satisfying the ceiling. If nothing in range fits
// under cap, it falls back to the lowest value in range: best effort,
// even if the cascade still won't be satisfied afterward (the caller's
// satisfiesTarget re-check handles that).
func relaxIntensityToFit(t activity.Type, current activity.IntensityLevel, cap activity.IntensityLevel) activity.IntensityLevel {
	best := activity.IntensityLevel("")
	bestRank := -1
	for _, l := range t.IntensityRange {
		if intensityRank[l] <= intensityRank[cap] && intensityRank[l] > bestRank {
			bestRank = intensityRank[l]
			best = l
		}
	}
	if bestRank == -1 {
		return lowestIntensity(t.IntensityRange)
	}
	return best
}

func relaxImpactToFit(t activity.Type, current activity.ImpactLevel, cap activity.ImpactLevel) activity.ImpactLevel {
	best := activity.ImpactLevel("")
	bestRank := -1
	for _, l := range t.ImpactRange {
		if impactRank[l] <= impactRank[cap] && impactRank[l] > bestRank {
			bestRank = impactRank[l]
			best = l
		}
	}
	if bestRank == -1 {
		return lowestImpact(t.ImpactRange)
	}
	return best
}

// ============================================================================
// SubstituteType
// ============================================================================

// substitute proposes a different activity type — catalog first, then
// (only if nothing in the catalog fits) the rest of the full taxonomy,
// flagged as "outside catalog, try something new" rather than failing
// outright. Only errors if truly nothing in the entire taxonomy can
// satisfy target, which VIV-102's "every goal has at least one primary
// priority" guarantee should make unreachable in practice.
func substitute(original Activity, catalog UserCatalog, target weeklytarget.WeeklyTarget) (SlotAssignment, error) {
	if sa, ok := findSubstitute(catalog.Activities, original, target, false); ok {
		return sa, nil
	}

	var outsideCatalog []activity.ID
	for _, t := range activity.Types {
		if !catalog.Has(t.ID) {
			outsideCatalog = append(outsideCatalog, t.ID)
		}
	}
	if sa, ok := findSubstitute(outsideCatalog, original, target, true); ok {
		return sa, nil
	}

	return SlotAssignment{}, fmt.Errorf("cascade: no substitute activity, in or out of catalog, satisfies the weekly target")
}

// findSubstitute tries candidates in order, picking the first activity
// type (other than the original) that has SOME intensity within
// target.IntensityCap producing a prioritized effect, and some impact
// within target.ImpactCap. Prefers the highest fitting intensity for that
// type, same reasoning as relaxIntensityToFit.
func findSubstitute(candidates []activity.ID, original Activity, target weeklytarget.WeeklyTarget, outsideCatalog bool) (SlotAssignment, bool) {
	for _, id := range candidates {
		if id == original.ActivityType {
			continue
		}
		t, ok := activity.ByID(id)
		if !ok {
			continue
		}

		bestIntensity := activity.IntensityLevel("")
		bestRank := -1
		for _, intensity := range t.IntensityRange {
			if intensityRank[intensity] > intensityRank[target.IntensityCap] {
				continue
			}
			if !producesAPrioritizedEffect(Activity{ActivityType: id, Intensity: intensity}, t, target.CompositionPriorities) {
				continue
			}
			if intensityRank[intensity] > bestRank {
				bestRank = intensityRank[intensity]
				bestIntensity = intensity
			}
		}
		if bestRank == -1 {
			continue // no intensity for this type both fits the cap and matches a priority
		}

		impact := activity.ImpactLevel("")
		impactBestRank := -1
		for _, l := range t.ImpactRange {
			if impactRank[l] <= impactRank[target.ImpactCap] && impactRank[l] > impactBestRank {
				impactBestRank = impactRank[l]
				impact = l
			}
		}
		if impactBestRank == -1 {
			continue // this type's impact range never fits the cap, regardless of intensity
		}

		reason := "substituted to fit this week's target"
		if outsideCatalog {
			reason = "outside your usual catalog — nothing in your current activities fit today's target, so this is a try-something-new suggestion"
		}

		return SlotAssignment{
			ActivityType:     id,
			Intensity:        bestIntensity,
			Impact:           impact,
			MuscleGroup:      t.DefaultMuscleGroup,
			Substituted:      true,
			OriginalType:     original.ActivityType,
			SubstituteReason: reason,
			LoadTier:         LoadStandard,
			DurationTier:     DurationStandard,
			ComplexityTier:   ComplexityStandard,
			AdjustmentLever:  AdjustmentSubstituteType,
		}, true
	}
	return SlotAssignment{}, false
}

// ============================================================================
// Substitution-cap fallback
// ============================================================================

// forcedRelax is the substitution-cap fallback (design doc §8.1: VIV won't
// substitute more than a small number of times in one week). Applied to
// the ORIGINAL candidate — not whatever partial state earlier levers in
// this same call left behind — and applies both RelaxImpact then
// RelaxIntensity unconditionally (whichever is applicable), regardless of
// whether either was already tried and failed earlier in this call. This
// is a distinct fallback, not a continuation of the normal cascade, so
// there's no satisfaction re-check afterward: it's the best available
// answer when substitution is off the table, whether or not it fully
// satisfies target.
func forcedRelax(original Activity, t activity.Type, target weeklytarget.WeeklyTarget) SlotAssignment {
	result := original

	if relaxImpactApplicable(t, result.Impact) {
		result.Impact = relaxImpactToFit(t, result.Impact, target.ImpactCap)
	}
	if relaxIntensityApplicable(t, result.Intensity) {
		result.Intensity = relaxIntensityToFit(t, result.Intensity, target.IntensityCap)
	}

	return SlotAssignment{
		ActivityType:    result.ActivityType,
		Intensity:       result.Intensity,
		Impact:          result.Impact,
		MuscleGroup:     result.MuscleGroup,
		OriginalType:    original.ActivityType,
		LoadTier:        LoadStandard,
		DurationTier:    DurationStandard,
		ComplexityTier:  ComplexityStandard,
		AdjustmentLever: AdjustmentForcedRelax,
	}
}
