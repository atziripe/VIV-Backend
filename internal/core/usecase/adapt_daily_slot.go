package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/cascade"
	"viv/internal/core/checkin"
	"viv/internal/core/goal"
	weeklytarget "viv/internal/core/weekly_target"
)

// ============================================================================
// ADAPT DAILY SLOT — VIV-107, the daily adaptation entry point (design doc §9)
// ============================================================================
//
// Depends on VIV-105 (cascade) and VIV-106 (the already-generated week this
// reads from). Deliberately separate from GenerateWeeklyPlanUsecase: this
// never regenerates a week, and it only ever touches the single slot for
// the given date.
type AdaptDailySlotUsecase struct {
	phaseLookup CyclePhaseLookup
	drafts      WeeklyPlanDraftRepository
}

func NewAdaptDailySlotUsecase(phaseLookup CyclePhaseLookup, drafts WeeklyPlanDraftRepository) *AdaptDailySlotUsecase {
	return &AdaptDailySlotUsecase{phaseLookup: phaseLookup, drafts: drafts}
}

type AdaptDailySlotInput struct {
	UserID  string
	Date    time.Time
	Checkin checkin.DailyCheckin

	// Catalog is optional — falls back to DefaultCatalog, same as
	// GenerateWeeklyPlanUsecase (VIV-106); see that type's doc comment for
	// why this isn't fetched through a repository.
	Catalog cascade.UserCatalog
}

// AdaptationSuggestion is a safety-relevant adjustment offered for a
// user-overridden slot — never applied automatically. The caller (UI)
// shows Reason and lets the user accept (via UserEditSlotUsecase) or
// dismiss it.
type AdaptationSuggestion struct {
	Assignment cascade.SlotAssignment
	Reason     string
}

type AdaptDailySlotOutput struct {
	Date time.Time

	// Assignment is what's actually scheduled for Date after this call —
	// unchanged from before the call unless Changed is true.
	Assignment cascade.SlotAssignment

	// Changed is true only when Assignment was auto-adjusted by the
	// cascade and persisted (the non-overridden path). It's always false
	// when the slot has UserOverrode set — see Suggestion instead.
	Changed bool

	// Reason explains Changed's adjustment in plain language, "" if
	// Changed is false.
	Reason string

	// Suggestion is non-nil only when the day's slot is user-overridden
	// AND a safety-relevant adjustment (RelaxIntensity/RelaxImpact) would
	// resolve today's mismatch. It is never applied by this function.
	Suggestion *AdaptationSuggestion
}

// Execute decides whether the date's already-scheduled session still fits
// today's fresh readiness, and if not, re-resolves just that one slot —
// see the package doc comment above for the full design-doc-driven
// decision tree (still-fits / not-overridden / overridden).
func (uc *AdaptDailySlotUsecase) Execute(ctx context.Context, input AdaptDailySlotInput) (AdaptDailySlotOutput, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: userID is required")
	}

	catalog := input.Catalog
	if len(catalog.Activities) == 0 {
		catalog = DefaultCatalog()
	}

	// ── Load the existing slot (VIV-106) — never regenerate the week ──
	draft, err := uc.drafts.GetByDate(ctx, userID, input.Date)
	if err != nil {
		return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: loading week: %w", err)
	}
	if draft == nil {
		return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: no generated week covers %s", input.Date.Format("2006-01-02"))
	}

	idx, ok := dayIndexForDate(*draft, input.Date)
	if !ok {
		return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: %s is not part of the loaded week", input.Date.Format("2006-01-02"))
	}

	day := draft.Days[idx]
	if day.IsRestDay {
		return AdaptDailySlotOutput{Date: day.Date, Assignment: day.Assignment, Changed: false}, nil
	}

	g, ok := goal.ByID(draft.GoalID)
	if !ok {
		return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: unknown goal id %q on the loaded week", draft.GoalID)
	}

	// ── VIV-103: today's fresh readiness ───────────────────────────
	readiness, err := checkin.Derive(input.Checkin)
	if err != nil {
		return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: deriving readiness: %w", err)
	}

	phase, err := uc.phaseLookup.CurrentPhase(ctx, userID)
	if err != nil {
		return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: looking up cycle phase: %w", err)
	}

	// ── VIV-104: today's fresh target — recomputed, not reused from
	// generation time, since readiness may have changed since then ──
	freshTarget, err := weeklytarget.BuildWeeklyTarget(
		g, readiness.RecoveryCapacity, readiness.LifeBandwidth, readiness.BuildReadiness, phase,
	)
	if err != nil {
		return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: building weekly target: %w", err)
	}

	existing := cascade.Activity{
		ActivityType: day.Assignment.ActivityType,
		Intensity:    day.Assignment.Intensity,
		Impact:       day.Assignment.Impact,
		MuscleGroup:  day.Assignment.MuscleGroup,
	}

	// ── Still fits? Return the existing assignment completely
	// untouched — no cascade run, no write ──────────────────────────
	stillFits, err := cascade.SatisfiesTarget(existing, freshTarget)
	if err != nil {
		return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: checking fit: %w", err)
	}
	if stillFits {
		return AdaptDailySlotOutput{Date: day.Date, Assignment: day.Assignment, Changed: false}, nil
	}

	// ── Doesn't fit anymore ─────────────────────────────────────────
	if !day.Assignment.UserOverrode {
		substitutionsUsed := countSubstitutionsExcluding(*draft, idx)

		resolved, err := cascade.ResolveSlotConflict(freshTarget, existing, catalog, g, substitutionsUsed)
		if err != nil {
			return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: resolving conflict: %w", err)
		}

		day.Assignment = resolved
		if err := uc.drafts.UpdateDaySlot(ctx, userID, draft.ID, idx, day); err != nil {
			return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: saving adjustment: %w", err)
		}

		return AdaptDailySlotOutput{
			Date:       day.Date,
			Assignment: resolved,
			Changed:    true,
			Reason:     AdjustmentReason(resolved.AdjustmentLever),
		}, nil
	}

	// User-overridden: only a safety-relevant suggestion is allowed —
	// design doc §9, "automatic adjustments explain, they never force."
	// The stored assignment is never touched here.
	suggested, ok, err := cascade.SuggestSafetyRelax(freshTarget, existing, g)
	if err != nil {
		return AdaptDailySlotOutput{}, fmt.Errorf("adapt daily slot: checking safety relax: %w", err)
	}

	out := AdaptDailySlotOutput{Date: day.Date, Assignment: day.Assignment, Changed: false}
	if ok {
		out.Suggestion = &AdaptationSuggestion{
			Assignment: suggested,
			Reason:     AdjustmentReason(suggested.AdjustmentLever),
		}
	}
	return out, nil
}

// countSubstitutionsExcluding counts how many of a week's other days
// already used a substitution — the weekly cap cascade.ResolveSlotConflict
// enforces — excluding the day currently being re-resolved (its old
// Substituted state is about to be replaced, not counted against itself).
func countSubstitutionsExcluding(draft WeekDraft, excludeIdx int) int {
	count := 0
	for i, d := range draft.Days {
		if i == excludeIdx {
			continue
		}
		if d.Assignment.Substituted {
			count++
		}
	}
	return count
}

// AdjustmentReason returns a plain-English, UI-ready explanation for a
// cascade.AdjustmentLever. This is deliberately NOT the full copy/
// localization layer (a later, separate concern) — just enough data for
// the UI to explain *why* an adjustment happened or is being suggested,
// per this task's scope.
func AdjustmentReason(lever cascade.AdjustmentLever) string {
	switch lever {
	case cascade.AdjustmentNone:
		return ""
	case cascade.AdjustmentProtectType:
		return "Kept your usual session type, but reduced the load to match how you're feeling today."
	case cascade.AdjustmentRelaxIntensity:
		return "Lowered the intensity to match how you're feeling today."
	case cascade.AdjustmentRelaxImpact:
		return "Reduced the impact on your joints to match how you're feeling today."
	case cascade.AdjustmentReduceDuration:
		return "Shortened today's session to match how you're feeling today."
	case cascade.AdjustmentSimplifyComplexity:
		return "Simplified today's session to match how you're feeling today."
	case cascade.AdjustmentSubstituteType:
		return "Swapped today's session for something that better fits how you're feeling today."
	case cascade.AdjustmentForcedRelax:
		return "You've already had a few swaps this week, so we adjusted today's session instead of substituting it again."
	default:
		return "Adjusted today's session to match how you're feeling today."
	}
}
