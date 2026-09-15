// Package weeklytarget implements the "0a" stage of the training-algorithm
// refactor described in "VIV Training Algorithm — Design Overview" (§7):
// combining a goal profile (VIV-102) with the day's readiness signals
// (VIV-103, derived from a daily check-in) into a WeeklyTarget — roughly
// what mix of training types makes sense, and what ceiling of intensity
// and impact is appropriate given how she's doing.
//
// This is still config/computation only: no conflict-resolution cascade,
// no matching against a user's actual catalog (design doc §7's second
// half, "checked against what the user actually practices") — that's
// VIV-105/106.
package weeklytarget

import (
	"fmt"

	"viv/internal/core/activity"
	"viv/internal/core/checkin"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
)

// ProgressionDirection mirrors checkin.BuildReadiness's three values but
// is its own type: WeeklyTarget is a downstream/output concept, and
// keeping it decoupled from checkin's input vocabulary means one can
// change without forcing the other, even though today the mapping
// between them is 1:1 (see progressionFromBuild).
type ProgressionDirection string

const (
	ProgressionPullBack    ProgressionDirection = "pull_back"
	ProgressionMaintain    ProgressionDirection = "maintain"
	ProgressionPushForward ProgressionDirection = "push_forward"
)

// WeeklyTarget is the "0a" stage's output — design doc §7.
type WeeklyTarget struct {
	// CompositionPriorities is threaded straight through from the goal
	// (VIV-102) — see TypeCompositionPriorities. Not resolved against
	// anything yet; that happens in VIV-105/106.
	CompositionPriorities []goal.CompositionPriority
	IntensityCap          activity.IntensityLevel
	ImpactCap             activity.ImpactLevel
	SessionBudget         int
	ProgressionDirection  ProgressionDirection
}

// TypeCompositionPriorities is a pass-through, not a computation: it
// returns the goal's own CompositionPriorities (VIV-102) unchanged. The
// actual *use* of these priorities — filling session slots Primary-effect
// first, then Supporting, then Occasional/Optional only if budget
// remains — happens in VIV-105/106 once slots get assigned against the
// user's catalog, not here.
func TypeCompositionPriorities(g goal.GoalProfile) []goal.CompositionPriority {
	return g.CompositionPriorities
}

// ReadinessResponseCurve turns the day's readiness signals into the
// week's intensity/impact ceiling, session budget, and progression
// direction — per goal, via a swappable strategy (the readinessCurve
// interface and the curves registry below), not a single switch
// statement, so one goal's curve can be retuned without touching the
// others.
//
// TODO(product/clinical): the exact numeric thresholds here (baseline
// session budgets per LifeBandwidth bucket, how many steps a pull_back
// drops the intensity/impact ceiling per goal) are reasonable
// placeholders, not a validated mapping — design doc §5 (the readiness
// signals themselves) and §7 (building the week's target) both leave
// this undefined today. Revisit before this backs a real user-facing
// plan; each curve implementation below notes the specific design-doc
// phrase its placeholder numbers are trying to reflect.
func ReadinessResponseCurve(
	g goal.GoalProfile,
	recovery checkin.RecoveryCapacity,
	bandwidth checkin.LifeBandwidth,
	build checkin.BuildReadiness,
) (activity.IntensityLevel, activity.ImpactLevel, int, ProgressionDirection, error) {
	if g.ID == "" {
		return "", "", 0, "", fmt.Errorf("weeklytarget: empty GoalProfile")
	}

	curve, ok := curves[g.ID]
	if !ok {
		return "", "", 0, "", fmt.Errorf("weeklytarget: no readiness curve registered for goal %q", g.ID)
	}

	out, err := curve.apply(recovery, bandwidth, build)
	if err != nil {
		return "", "", 0, "", err
	}

	return out.IntensityCap, out.ImpactCap, out.SessionBudget, out.ProgressionDirection, nil
}

// BuildWeeklyTarget is the full "0a" stage: TypeCompositionPriorities +
// ReadinessResponseCurve combined into one WeeklyTarget.
//
// phase is accepted and threaded through for future use/logging only —
// design doc §6: cycle phase provides context the readiness signals
// already reflect (a symptomatic day should already show up in the
// Sleep/Body/Demand answers), so per the design it deliberately does NOT
// branch any logic here. No phase-conditional behavior — that line isn't
// meant to move even later, per §6.
func BuildWeeklyTarget(
	g goal.GoalProfile,
	recovery checkin.RecoveryCapacity,
	bandwidth checkin.LifeBandwidth,
	build checkin.BuildReadiness,
	phase domain.CyclePhase,
) (WeeklyTarget, error) {
	intensityCap, impactCap, budget, direction, err := ReadinessResponseCurve(g, recovery, bandwidth, build)
	if err != nil {
		return WeeklyTarget{}, err
	}

	return WeeklyTarget{
		CompositionPriorities: TypeCompositionPriorities(g),
		IntensityCap:          intensityCap,
		ImpactCap:             impactCap,
		SessionBudget:         budget,
		ProgressionDirection:  direction,
	}, nil
}

// ============================================================================
// Per-goal readiness curves — the swappable strategy.
// ============================================================================

type curveOutput struct {
	IntensityCap         activity.IntensityLevel
	ImpactCap            activity.ImpactLevel
	SessionBudget        int
	ProgressionDirection ProgressionDirection
}

type readinessCurve interface {
	apply(recovery checkin.RecoveryCapacity, bandwidth checkin.LifeBandwidth, build checkin.BuildReadiness) (curveOutput, error)
}

// curves is the registry driving ReadinessResponseCurve's dispatch — add
// or retune a goal's curve here, never by adding a branch to
// ReadinessResponseCurve itself.
var curves = map[goal.ID]readinessCurve{
	// "Reduce load or volume while preserving a useful strength stimulus"
	// (design doc §4, Strength & muscle's Lower-readiness row) — protect
	// session count, ease off intensity only, one step.
	goal.StrengthMuscle: protectStimulusCurve{intensityDropOnPullBack: 1},
	// "Replace high intensity with easier aerobic work, technique or
	// recovery" (design doc §4, Endurance & performance's Lower-readiness
	// row) — a bigger intensity concession than Strength's, since the
	// doc's own wording goes further ("replace", not "reduce"); still
	// protects session count and impact ceiling.
	goal.EndurancePerformance: protectStimulusCurve{intensityDropOnPullBack: 2},
	// "Maintain the muscle stimulus without adding excessive fatigue"
	// (design doc §4, Body composition's Lower-readiness row) — this
	// goal's own ConflictLeverOrder tries RelaxImpact before
	// RelaxIntensity (VIV-102), so its readiness curve leans on the same
	// dimension first, plus a modest session-budget trim for "excessive
	// fatigue" management.
	goal.BodyComposition: balanceCurve{},
	// "Lower the barrier: shorten, simplify or reduce intensity rather
	// than skip" (design doc §4, Consistency & wellbeing's Lower-readiness
	// row) — the most aggressive ceiling reduction (both intensity AND
	// impact drop), but session budget is deliberately untouched: this
	// goal's whole point is never to cut a session, only to make it
	// easier.
	goal.ConsistencyWellbeing: lowerBarrierCurve{},
}

// protectStimulusCurve backs the two goals with a ProtectedActivityType
// (VIV-102): on a pull_back, it only relaxes the intensity ceiling —
// impact ceiling and session budget are untouched, preserving both the
// session count and how much joint loading is allowed.
type protectStimulusCurve struct {
	intensityDropOnPullBack int
}

func (c protectStimulusCurve) apply(recovery checkin.RecoveryCapacity, bandwidth checkin.LifeBandwidth, build checkin.BuildReadiness) (curveOutput, error) {
	intensity, impact, budget, direction, err := baseline(recovery, bandwidth, build)
	if err != nil {
		return curveOutput{}, err
	}

	if build == checkin.BuildPullBack {
		intensity = dropIntensity(intensity, c.intensityDropOnPullBack)
	}

	return curveOutput{IntensityCap: intensity, ImpactCap: impact, SessionBudget: budget, ProgressionDirection: direction}, nil
}

// balanceCurve backs Body composition — the one goal whose own
// ConflictLeverOrder starts with RelaxImpact rather than RelaxIntensity,
// so its readiness curve mirrors that: impact ceiling relaxes first, and
// the session budget takes a modest cut too.
type balanceCurve struct{}

func (c balanceCurve) apply(recovery checkin.RecoveryCapacity, bandwidth checkin.LifeBandwidth, build checkin.BuildReadiness) (curveOutput, error) {
	intensity, impact, budget, direction, err := baseline(recovery, bandwidth, build)
	if err != nil {
		return curveOutput{}, err
	}

	if build == checkin.BuildPullBack {
		impact = dropImpact(impact, 1)
		if budget > 1 {
			budget--
		}
	}

	return curveOutput{IntensityCap: intensity, ImpactCap: impact, SessionBudget: budget, ProgressionDirection: direction}, nil
}

// lowerBarrierCurve backs Consistency & wellbeing — relaxes intensity AND
// impact together on a pull_back (a bigger ceiling concession than either
// of the other curves), but never touches the session budget: this goal
// shortens/simplifies rather than skips (design doc §4).
type lowerBarrierCurve struct{}

func (c lowerBarrierCurve) apply(recovery checkin.RecoveryCapacity, bandwidth checkin.LifeBandwidth, build checkin.BuildReadiness) (curveOutput, error) {
	intensity, impact, budget, direction, err := baseline(recovery, bandwidth, build)
	if err != nil {
		return curveOutput{}, err
	}

	if build == checkin.BuildPullBack {
		intensity = dropIntensity(intensity, 1)
		impact = dropImpact(impact, 1)
	}

	return curveOutput{IntensityCap: intensity, ImpactCap: impact, SessionBudget: budget, ProgressionDirection: direction}, nil
}

// baseline computes the shared starting point every curve adjusts from:
// intensity/impact ceilings straight from RecoveryCapacity, session
// budget straight from LifeBandwidth, and the direct BuildReadiness →
// ProgressionDirection mapping. Goal-specific behavior only ever adjusts
// DOWN from here (design doc §8's adjustment levers are all concessions,
// never upgrades) — see each curve's apply() for what it changes.
func baseline(
	recovery checkin.RecoveryCapacity,
	bandwidth checkin.LifeBandwidth,
	build checkin.BuildReadiness,
) (activity.IntensityLevel, activity.ImpactLevel, int, ProgressionDirection, error) {
	intensity, impact, err := baselineFromRecovery(recovery)
	if err != nil {
		return "", "", 0, "", err
	}
	budget, err := baselineSessionBudget(bandwidth)
	if err != nil {
		return "", "", 0, "", err
	}
	direction, err := progressionFromBuild(build)
	if err != nil {
		return "", "", 0, "", err
	}
	return intensity, impact, budget, direction, nil
}

func baselineFromRecovery(recovery checkin.RecoveryCapacity) (activity.IntensityLevel, activity.ImpactLevel, error) {
	switch recovery {
	case checkin.RecoveryHigh:
		return activity.IntensityH, activity.ImpactH, nil
	case checkin.RecoveryModerate:
		return activity.IntensityM, activity.ImpactM, nil
	case checkin.RecoveryLow:
		return activity.IntensityL, activity.ImpactL, nil
	default:
		return "", "", fmt.Errorf("weeklytarget: invalid RecoveryCapacity %q", recovery)
	}
}

func baselineSessionBudget(bandwidth checkin.LifeBandwidth) (int, error) {
	switch bandwidth {
	case checkin.BandwidthHigh:
		return 5, nil
	case checkin.BandwidthModerate:
		return 4, nil
	case checkin.BandwidthLow:
		return 2, nil
	default:
		return 0, fmt.Errorf("weeklytarget: invalid LifeBandwidth %q", bandwidth)
	}
}

func progressionFromBuild(build checkin.BuildReadiness) (ProgressionDirection, error) {
	switch build {
	case checkin.BuildPushForward:
		return ProgressionPushForward, nil
	case checkin.BuildMaintain:
		return ProgressionMaintain, nil
	case checkin.BuildPullBack:
		return ProgressionPullBack, nil
	default:
		return "", fmt.Errorf("weeklytarget: invalid BuildReadiness %q", build)
	}
}

var intensityOrder = []activity.IntensityLevel{activity.IntensityL, activity.IntensityM, activity.IntensityH}
var impactOrder = []activity.ImpactLevel{activity.ImpactL, activity.ImpactM, activity.ImpactH}

// dropIntensity steps an intensity ceiling down, clamped at the floor
// (L). AR isn't part of this ordering — it's specific to Yoga/Mobility's
// own IntensityRange (VIV-101), not a general week-level ceiling.
func dropIntensity(level activity.IntensityLevel, steps int) activity.IntensityLevel {
	idx := 0
	for i, l := range intensityOrder {
		if l == level {
			idx = i
			break
		}
	}
	idx -= steps
	if idx < 0 {
		idx = 0
	}
	return intensityOrder[idx]
}

func dropImpact(level activity.ImpactLevel, steps int) activity.ImpactLevel {
	idx := 0
	for i, l := range impactOrder {
		if l == level {
			idx = i
			break
		}
	}
	idx -= steps
	if idx < 0 {
		idx = 0
	}
	return impactOrder[idx]
}
