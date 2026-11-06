package usecase

import (
	"context"
	"fmt"
	"strings"

	"viv/internal/core/scheduling"
)

// ============================================================================
// RULE ENGINE VALIDATOR — VIV-110, design doc's "Rule Engine"
// ============================================================================
//
// Explored the existing Rule Engine (internal/core/rules — Engine,
// Decision, Violation, the old system's Modality/Intensity validator) and
// VIV-109's scheduling package before writing this. The old engine
// evaluates a domain.WeekArrangement against a registered []Rule set and
// is wired into the OLD pipeline (usecase.GenerateTrainingPlanUsecase);
// this is deliberately not a reuse of it — the new pipeline's slots are
// already fully resolved before scheduling ever runs (VIV-105), so there
// is nothing left to validate here except whether the LLM's day
// PLACEMENT (VIV-109) respects the same spacing rules scheduling itself
// already encodes (internal/core/scheduling.CheckSpacing). This is a
// safety net, not a second opinion on a different rule set.
//
// RuleEngineValidator implements usecase.ValidationEngine (the "Layer
// after Layer 1" seam GenerateWeeklyPlanUsecase already wires generically
// — see weekly_plan_layers.go).
type RuleEngineValidator struct {
	scheduler SchedulingLayer
}

func NewRuleEngineValidator(scheduler SchedulingLayer) *RuleEngineValidator {
	return &RuleEngineValidator{scheduler: scheduler}
}

// Validate re-checks a scheduled week against scheduling.CheckSpacing. If
// it passes, the draft is returned unchanged — no retry, no LLM call. If
// it fails, Layer 1 (SchedulingLayer) is called exactly once more with
// feedback describing exactly what was violated, using the week's real
// weekday labels (e.g. "Monday and Tuesday are both Impact=High"). If the
// retried schedule still violates spacing, this returns a hard error —
// see the TODO below — rather than accepting an invalid plan.
func (v *RuleEngineValidator) Validate(ctx context.Context, draft WeekDraft) (WeekDraft, error) {
	violations := scheduling.CheckSpacing(toSchedulingDays(draft))
	if len(violations) == 0 {
		return draft, nil
	}

	feedback := formatSpacingFeedback(draft, violations)

	retried, err := v.scheduler.Schedule(ctx, draft, feedback)
	if err != nil {
		return WeekDraft{}, fmt.Errorf("rule engine: retrying Layer 1 scheduling after a spacing violation: %w", err)
	}

	stillViolations := scheduling.CheckSpacing(toSchedulingDays(retried))
	if len(stillViolations) > 0 {
		// TODO(product, Design Overview §11): what happens when a
		// rescheduled week STILL violates spacing after this one retry —
		// relax the rule automatically? escalate to manual review? fall
		// back to a simpler template? — is an explicitly undecided
		// product question. Do not guess at an answer here: surface a
		// hard error so the caller (GenerateWeeklyPlanUsecase) has to
		// handle this case explicitly rather than it being swallowed or
		// an invalid plan being silently accepted.
		return WeekDraft{}, fmt.Errorf(
			"rule engine: scheduled week still violates spacing rules after one retry (%d violation(s)); refusing to accept an invalid plan — see Design Overview §11 for the undecided fallback policy. Violations:\n%s",
			len(stillViolations), formatSpacingFeedback(retried, stillViolations),
		)
	}

	return retried, nil
}

// toSchedulingDays adapts a WeekDraft to scheduling.CheckSpacing's input
// shape.
func toSchedulingDays(draft WeekDraft) [7]scheduling.DayPlacement {
	var days [7]scheduling.DayPlacement
	for i, d := range draft.Days {
		days[i] = scheduling.DayPlacement{IsRestDay: d.IsRestDay, Assignment: d.Assignment}
	}
	return days
}

// formatSpacingFeedback turns structured scheduling.Violation data into
// the specific, human-readable feedback Layer 1's retry prompt needs —
// e.g. "Monday and Tuesday are both Impact=High" or "Lower-body sessions
// on Monday and Tuesday violate the recovery gap" — using the draft's
// real weekday labels (draft.Days[i].Weekday) rather than raw day indices.
func formatSpacingFeedback(draft WeekDraft, violations []scheduling.Violation) string {
	var b strings.Builder
	b.WriteString("Your previous scheduling proposal violated these spacing rules:\n")

	for i, v := range violations {
		dayLabels := make([]string, len(v.AffectedDays))
		for j, idx := range v.AffectedDays {
			dayLabels[j] = weekdayLabel(draft, idx)
		}

		switch v.Rule {
		case scheduling.RuleConsecutiveHighImpact:
			b.WriteString(fmt.Sprintf("(%d) %s and %s are both Impact=High — no two consecutive days may both be High impact.\n",
				i+1, dayLabels[0], dayLabels[1]))
		case scheduling.RuleRecoveryGap:
			b.WriteString(fmt.Sprintf("(%d) %s-body sessions on %s and %s violate the recovery gap for muscle group %q.\n",
				i+1, titleCase(string(v.MuscleGroup)), dayLabels[0], dayLabels[1], v.MuscleGroup))
		default:
			b.WriteString(fmt.Sprintf("(%d) days %v violate a spacing rule (%s).\n", i+1, v.AffectedDays, v.Rule))
		}
	}

	b.WriteString("\nPlease choose different days that respect these constraints. Do not change any session's activity type, intensity, impact, or muscle group — only which day it's on.")
	return b.String()
}

func weekdayLabel(draft WeekDraft, dayIndex int) string {
	if dayIndex < 0 || dayIndex >= len(draft.Days) {
		return fmt.Sprintf("day %d", dayIndex)
	}
	return titleCase(draft.Days[dayIndex].Weekday)
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
