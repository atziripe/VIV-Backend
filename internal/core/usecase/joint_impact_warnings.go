package usecase

import (
	"context"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
)

// ============================================================================
// JOINT IMPACT WARNINGS — VIV-111, design doc's "Layer 2"
// ============================================================================
//
// Explored the existing Layer 2 (internal/core/training/transformations.go)
// before writing this: the old ApplyTransformations is the same shape —
// purely additive annotations (LoadLabel, JointWarning) computed from a
// session's already-final Modality/MuscleGroup and the cycle phase, never
// changing the session itself. This is the new pipeline's equivalent,
// computed from VIV-101/105 data (a slot's resolved Impact) instead of
// cycle phase, and implements usecase.WarningsOverridesLayer — the
// interface's shape (Apply(ctx, draft) (WeekDraft, error)) already
// enforces "annotate, don't rebuild": there's no way to construct a new
// WeekDraft here without threading every existing field through.
//
// JointImpactWarningsLayer has exactly two responsibilities per the task:
//
//  1. Joint-impact warning: any non-rest-day slot whose resolved Impact
//     crosses JointImpactWarningThreshold gets DayPlan.Warning set to a
//     placeholder message — never touches ActivityType, Intensity,
//     Impact, MuscleGroup, or which day a session falls on.
//  2. Load override: SlotAssignment.LoadTier (set by VIV-105's cascade
//     when the ProtectType lever fires) needs no new logic here — it
//     already lives on cascade.SlotAssignment, and this layer (like
//     every other stage since VIV-106 assembled the draft) never
//     mutates Assignment at all, only ever DayPlan.Warning. See
//     TestJointImpactWarningsLayer_LoadTierSurvivesUnchanged, which is
//     this task's "verify and test" half.
type JointImpactWarningsLayer struct{}

func NewJointImpactWarningsLayer() JointImpactWarningsLayer {
	return JointImpactWarningsLayer{}
}

// Apply implements usecase.WarningsOverridesLayer.
func (JointImpactWarningsLayer) Apply(_ context.Context, draft WeekDraft) (WeekDraft, error) {
	for i, d := range draft.Days {
		if d.IsRestDay {
			continue
		}
		draft.Days[i].Warning = jointImpactWarningFor(d.Assignment)
	}
	return draft, nil
}

// JointImpactWarningThreshold says which resolved Impact levels trigger a
// joint-impact warning. High is confirmed to warrant one.
//
// TODO(product/clinical): whether Medium impact should also trigger a
// warning is unconfirmed — Design Overview doesn't pin this down.
// Configurable here (not an inline `if impact == activity.ImpactH`) so
// flipping Medium to true is a one-place change once this is decided.
var JointImpactWarningThreshold = map[activity.ImpactLevel]bool{
	activity.ImpactH: true,
	activity.ImpactM: false, // TODO(product/clinical): confirm whether Medium impact also warrants a warning
	activity.ImpactL: false,
}

// jointImpactWarningPlaceholder is deliberately generic engineering
// placeholder copy — final warning copy is a content/clinical task, not
// an engineering one.
const jointImpactWarningPlaceholder = "This session carries a higher level of joint impact — placeholder copy, pending clinical/content review."

func jointImpactWarningFor(a cascade.SlotAssignment) string {
	if !JointImpactWarningThreshold[a.Impact] {
		return ""
	}
	return jointImpactWarningPlaceholder
}
