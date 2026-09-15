package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/checkin"
	"viv/internal/core/goal"
	weeklytarget "viv/internal/core/weekly_target"
)

// ============================================================================
// GENERATE WEEKLY PLAN — the new-system orchestrator (depends on VIV-104, VIV-105)
// ============================================================================
//
// Sequence: readiness (VIV-103) → 0a WeeklyTarget (VIV-104) → per-day 0b
// ResolveSlotConflict (VIV-105) → stub Layer 1/RuleEngine/Layer 2/Layer 3
// (VIV-109..112, not built yet) → persist as an editable draft.
//
// This is deliberately a separate entry point from daily adaptation
// (VIV-107) — it only ever produces a fresh 7-day draft.
type GenerateWeeklyPlanUsecase struct {
	phaseLookup CyclePhaseLookup
	scheduling  SchedulingLayer
	validation  ValidationEngine
	overrides   WarningsOverridesLayer
	content     ContentSelectionLayer
	drafts      WeeklyPlanDraftRepository
}

func NewGenerateWeeklyPlanUsecase(
	phaseLookup CyclePhaseLookup,
	scheduling SchedulingLayer,
	validation ValidationEngine,
	overrides WarningsOverridesLayer,
	content ContentSelectionLayer,
	drafts WeeklyPlanDraftRepository,
) *GenerateWeeklyPlanUsecase {
	return &GenerateWeeklyPlanUsecase{
		phaseLookup: phaseLookup,
		scheduling:  scheduling,
		validation:  validation,
		overrides:   overrides,
		content:     content,
		drafts:      drafts,
	}
}

// GenerateWeeklyPlanInput contains everything needed to generate a week.
//
// Checkin, GoalID, and Catalog are all optional (zero-value-able) and fall
// back to a documented default when omitted — see DefaultDailyCheckin,
// DefaultGoalID, and DefaultCatalog. None of the three has a real
// persisted source yet in this codebase (no daily-checkin store, no goal
// onboarding, no catalog onboarding), so rather than inventing a repository
// that would quietly fabricate "real" data, the caller supplies what it has
// and the orchestrator is explicit about what it assumed when it doesn't.
type GenerateWeeklyPlanInput struct {
	UserID         string
	GenerationDate time.Time // the first day of the 7-day span this plan covers

	Checkin *checkin.DailyCheckin
	GoalID  goal.ID
	Catalog cascade.UserCatalog

	// Readiness, when non-nil, is used as-is instead of being derived from
	// Checkin — for callers that have no daily check-in to derive from at
	// all (e.g. onboarding's first-ever generation; see
	// DefaultOnboardingReadiness in complete_onboarding.go). Checkin is
	// ignored when this is set.
	Readiness *checkin.ReadinessDimensions
}

type GenerateWeeklyPlanOutput struct {
	Draft WeekDraft
}

// DefaultDailyCheckin is applied when GenerateWeeklyPlanInput.Checkin is
// nil — e.g. a brand-new user's first-ever week, before any check-in
// exists. It reads as "an unremarkable, unstressed day" on every
// dimension: deliberately neutral rather than optimistic or pessimistic,
// so a first-time generation doesn't over- or under-shoot.
var DefaultDailyCheckin = checkin.DailyCheckin{
	Sleep:  checkin.SleepNormal,
	Body:   checkin.BodyNormal,
	Demand: checkin.DemandNormal,
	Need:   checkin.NeedMeetMeWhereImAt,
}

// DefaultGoalID is applied when GenerateWeeklyPlanInput.GoalID is "".
// consistency_wellbeing is chosen deliberately as the lowest-commitment,
// safest default — it never assumes the user wants aggressive
// progression. Real goal selection is an onboarding concern with no
// ticket yet.
const DefaultGoalID = goal.ConsistencyWellbeing

// DefaultCatalog is applied when GenerateWeeklyPlanInput.Catalog has no
// Activities. It offers the full activity taxonomy — this is a first-draft
// placeholder, NOT a model of real catalog selection; real catalog
// onboarding/preferences is a separate, not-yet-scoped feature. Documented
// here rather than silently assumed.
func DefaultCatalog() cascade.UserCatalog {
	ids := make([]activity.ID, len(activity.Types))
	for i, t := range activity.Types {
		ids[i] = t.ID
	}
	return cascade.UserCatalog{Activities: ids}
}

func (uc *GenerateWeeklyPlanUsecase) Execute(
	ctx context.Context,
	input GenerateWeeklyPlanInput,
) (GenerateWeeklyPlanOutput, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return GenerateWeeklyPlanOutput{}, fmt.Errorf("weekly plan: userID is required")
	}

	goalID := input.GoalID
	if goalID == "" {
		goalID = DefaultGoalID
	}
	g, ok := goal.ByID(goalID)
	if !ok {
		return GenerateWeeklyPlanOutput{}, fmt.Errorf("weekly plan: unknown goal id %q", goalID)
	}

	catalog := input.Catalog
	if len(catalog.Activities) == 0 {
		catalog = DefaultCatalog()
	}

	// ── VIV-103: readiness dimensions ────────────────────────────
	//
	// Either taken as-is from an explicit override, or derived from a
	// daily check-in (falling back to DefaultDailyCheckin when none is
	// given) — see GenerateWeeklyPlanInput.Readiness.
	var readiness checkin.ReadinessDimensions
	if input.Readiness != nil {
		readiness = *input.Readiness
	} else {
		dailyCheckin := DefaultDailyCheckin
		if input.Checkin != nil {
			dailyCheckin = *input.Checkin
		}
		var err error
		readiness, err = checkin.Derive(dailyCheckin)
		if err != nil {
			return GenerateWeeklyPlanOutput{}, fmt.Errorf("weekly plan: deriving readiness: %w", err)
		}
	}

	phase, err := uc.phaseLookup.CurrentPhase(ctx, userID)
	if err != nil {
		return GenerateWeeklyPlanOutput{}, fmt.Errorf("weekly plan: looking up cycle phase: %w", err)
	}

	// ── VIV-104: 0a — this week's target ─────────────────────────
	target, err := weeklytarget.BuildWeeklyTarget(
		g, readiness.RecoveryCapacity, readiness.LifeBandwidth, readiness.BuildReadiness, phase,
	)
	if err != nil {
		return GenerateWeeklyPlanOutput{}, fmt.Errorf("weekly plan: building weekly target: %w", err)
	}

	draft := WeekDraft{
		UserID:         userID,
		GenerationDate: input.GenerationDate,
		StartDate:      input.GenerationDate,
		EndDate:        input.GenerationDate.AddDate(0, 0, 6),
		Status:         "draft",
		GoalID:         goalID,
		Readiness:      readiness,
		Target:         target,
	}

	// ── VIV-105: 0b — per-day candidate + conflict resolution ────
	//
	// Which days train vs. rest: the first target.SessionBudget days of
	// the span are training days, the rest are rest days. A simple,
	// deterministic rule — real day-of-week placement (e.g. spreading
	// sessions out, respecting user scheduling preferences) is Layer 1's
	// job (VIV-109), not this orchestrator's.
	//
	// Which activity type per training day: rotate through the user's
	// catalog in order. Deliberately not smarter than that — richer
	// catalog-selection logic is out of this task's scope.
	//
	// Proposed intensity/impact per candidate: each type's own highest
	// in-range value, representing the undiminished "ideal" session for
	// that type. ResolveSlotConflict (VIV-105) is what brings it down to
	// fit the week's actual, readiness-driven target.
	substitutionsUsed := 0
	catalogIdx := 0
	trainingDaysAssigned := 0

	for i := 0; i < 7; i++ {
		date := input.GenerationDate.AddDate(0, 0, i)
		day := DayPlan{
			Date:    date,
			Weekday: strings.ToLower(date.Weekday().String()),
		}

		if trainingDaysAssigned >= target.SessionBudget {
			day.IsRestDay = true
			draft.Days[i] = day
			continue
		}

		activityID := catalog.Activities[catalogIdx%len(catalog.Activities)]
		catalogIdx++
		trainingDaysAssigned++

		t, ok := activity.ByID(activityID)
		if !ok {
			return GenerateWeeklyPlanOutput{}, fmt.Errorf(
				"weekly plan: catalog references unknown activity type %q", activityID,
			)
		}

		candidate := cascade.Activity{
			ActivityType: activityID,
			Intensity:    highestIntensity(t.IntensityRange),
			Impact:       highestImpact(t.ImpactRange),
			MuscleGroup:  t.DefaultMuscleGroup,
		}

		assignment, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, substitutionsUsed)
		if err != nil {
			return GenerateWeeklyPlanOutput{}, fmt.Errorf(
				"weekly plan: resolving %s (day %d): %w", day.Weekday, i+1, err,
			)
		}
		if assignment.Substituted {
			substitutionsUsed++
		}

		day.Assignment = assignment
		draft.Days[i] = day
	}

	// ── Stub Layer 1 / Rule Engine / Layer 2 / Layer 3 ───────────
	// Real implementations arrive in VIV-109..112; each stage is called
	// through its interface so it can be swapped independently later.
	draft, err = uc.scheduling.Schedule(ctx, draft, "")
	if err != nil {
		return GenerateWeeklyPlanOutput{}, fmt.Errorf("weekly plan: scheduling layer: %w", err)
	}
	draft, err = uc.validation.Validate(ctx, draft)
	if err != nil {
		return GenerateWeeklyPlanOutput{}, fmt.Errorf("weekly plan: rule engine validation: %w", err)
	}
	draft, err = uc.overrides.Apply(ctx, draft)
	if err != nil {
		return GenerateWeeklyPlanOutput{}, fmt.Errorf("weekly plan: warnings/overrides layer: %w", err)
	}
	draft, err = uc.content.SelectContent(ctx, draft)
	if err != nil {
		return GenerateWeeklyPlanOutput{}, fmt.Errorf("weekly plan: content selection layer: %w", err)
	}

	// ── Persist as an editable draft ─────────────────────────────
	draft.CreatedAt = time.Now().UTC()
	if err := uc.drafts.SaveDraft(ctx, &draft); err != nil {
		return GenerateWeeklyPlanOutput{}, fmt.Errorf("weekly plan: saving draft: %w", err)
	}

	return GenerateWeeklyPlanOutput{Draft: draft}, nil
}

// ============================================================================
// Local intensity/impact ranking — for proposing each day's "ideal"
// candidate before the cascade relaxes it. Deliberately not shared with
// internal/core/cascade's own (unexported) ranking: this is a
// candidate-proposal heuristic that belongs to the orchestrator, not a
// fact about the taxonomy itself.
// ============================================================================

var intensityRank = map[activity.IntensityLevel]int{
	activity.IntensityAR: 0, activity.IntensityL: 1, activity.IntensityM: 2, activity.IntensityH: 3,
}
var impactRank = map[activity.ImpactLevel]int{
	activity.ImpactL: 0, activity.ImpactM: 1, activity.ImpactH: 2,
}

func highestIntensity(levels []activity.IntensityLevel) activity.IntensityLevel {
	best := levels[0]
	for _, l := range levels[1:] {
		if intensityRank[l] > intensityRank[best] {
			best = l
		}
	}
	return best
}

func highestImpact(levels []activity.ImpactLevel) activity.ImpactLevel {
	best := levels[0]
	for _, l := range levels[1:] {
		if impactRank[l] > impactRank[best] {
			best = l
		}
	}
	return best
}
