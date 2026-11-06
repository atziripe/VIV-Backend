// Package checkin models VIV's daily check-in (design doc "VIV Training
// Algorithm — Design Overview", §5) and the pure translation from its four
// answers into the three readiness dimensions the rest of the new
// training algorithm consumes.
//
// This supersedes, for the new algorithm, the existing weekly check-in
// translator (domain.Checkin + training.TranslateCheckin) — a different
// answer vocabulary (human-readable labels like "Consistent"/"Manageable"
// vs. this package's snake_case values), a different cadence (weekly vs.
// daily), and critically, silent defaulting: training.TranslateCheckin
// falls back to defaultDimensions() for an empty or unrecognized checkin
// instead of erroring. Derive below never does that — see its doc
// comment. The old translator is untouched here; it still backs the
// current production training engine.
package checkin

import "fmt"

// SleepAnswer — "How did you sleep last night?"
type SleepAnswer string

const (
	SleepDeepAndRestful SleepAnswer = "deep_and_restful"
	SleepNormal         SleepAnswer = "normal"
	SleepRestless       SleepAnswer = "restless"
	SleepBarelySlept    SleepAnswer = "barely_slept"
)

// BodyAnswer — "How does your body feel?"
type BodyAnswer string

const (
	BodyStrongAndResponsive BodyAnswer = "strong_and_responsive"
	BodyNormal              BodyAnswer = "normal"
	BodyHeavierThanUsual    BodyAnswer = "heavier_than_usual"
	BodySensitiveOrReactive BodyAnswer = "sensitive_or_reactive"
)

// DemandAnswer — "How demanding is today?"
type DemandAnswer string

const (
	DemandLightAndOpen  DemandAnswer = "light_and_open"
	DemandNormal        DemandAnswer = "normal"
	DemandPacked        DemandAnswer = "packed"
	DemandUnpredictable DemandAnswer = "unpredictable"
)

// NeedAnswer — "What do you need from today's session?"
type NeedAnswer string

const (
	NeedPushMe          NeedAnswer = "push_me"
	NeedMeetMeWhereImAt NeedAnswer = "meet_me_where_im_at"
	NeedLetMeReset      NeedAnswer = "let_me_reset"
)

// DailyCheckin is the four raw answers, exactly as submitted.
type DailyCheckin struct {
	Sleep  SleepAnswer
	Body   BodyAnswer
	Demand DemandAnswer
	Need   NeedAnswer
}

// RecoveryCapacity — design doc §5.1.
type RecoveryCapacity string

const (
	RecoveryLow      RecoveryCapacity = "low"
	RecoveryModerate RecoveryCapacity = "moderate"
	RecoveryHigh     RecoveryCapacity = "high"
)

// LifeBandwidth — design doc §5.2.
type LifeBandwidth string

const (
	BandwidthLow      LifeBandwidth = "low"
	BandwidthModerate LifeBandwidth = "moderate"
	BandwidthHigh     LifeBandwidth = "high"
)

// BuildReadiness — design doc §5.3.
type BuildReadiness string

const (
	BuildPullBack    BuildReadiness = "pull_back"
	BuildMaintain    BuildReadiness = "maintain"
	BuildPushForward BuildReadiness = "push_forward"
)

// ReadinessDimensions is the derived output — what goal profiles and,
// later, the conflict-resolution cascade (VIV-105) actually consume.
type ReadinessDimensions struct {
	RecoveryCapacity RecoveryCapacity
	LifeBandwidth    LifeBandwidth
	BuildReadiness   BuildReadiness
}

// Derive converts a daily check-in into its three readiness dimensions.
// Pure and deterministic — no LLM, no I/O, no side effects.
//
// Returns an explicit error for any unrecognized answer value instead of
// silently defaulting: a malformed check-in answer must never resolve to
// a plausible-looking readiness score, since that score directly drives
// what training a user is told to do that day.
func Derive(c DailyCheckin) (ReadinessDimensions, error) {
	sleepPts, err := sleepPoints(c.Sleep)
	if err != nil {
		return ReadinessDimensions{}, err
	}
	bodyPts, err := bodyPoints(c.Body)
	if err != nil {
		return ReadinessDimensions{}, err
	}

	bandwidth, err := lifeBandwidth(c.Demand)
	if err != nil {
		return ReadinessDimensions{}, err
	}

	build, err := buildReadiness(c.Need)
	if err != nil {
		return ReadinessDimensions{}, err
	}

	return ReadinessDimensions{
		RecoveryCapacity: recoveryFromScore(sleepPts + bodyPts),
		LifeBandwidth:    bandwidth,
		BuildReadiness:   build,
	}, nil
}

// sleepPoints — design doc §5.1: deep_and_restful=+2, normal=+1,
// restless=0, barely_slept=-1.
func sleepPoints(a SleepAnswer) (int, error) {
	switch a {
	case SleepDeepAndRestful:
		return 2, nil
	case SleepNormal:
		return 1, nil
	case SleepRestless:
		return 0, nil
	case SleepBarelySlept:
		return -1, nil
	default:
		return 0, fmt.Errorf("checkin: invalid sleep answer %q", a)
	}
}

// bodyPoints — design doc §5.1: strong_and_responsive=+2, normal=+1,
// heavier_than_usual=0, sensitive_or_reactive=-1.
func bodyPoints(a BodyAnswer) (int, error) {
	switch a {
	case BodyStrongAndResponsive:
		return 2, nil
	case BodyNormal:
		return 1, nil
	case BodyHeavierThanUsual:
		return 0, nil
	case BodySensitiveOrReactive:
		return -1, nil
	default:
		return 0, fmt.Errorf("checkin: invalid body answer %q", a)
	}
}

// recoveryFromScore — design doc §5.1: sum 3-4 → High, 1-2 → Moderate,
// ≤0 → Low. The sum ranges -2..4 (two questions, each -1..2); every value
// in that range is covered by exactly one of these three branches, so
// there's no unreachable/undefined score to worry about here — the
// invalid-input surface is entirely at sleepPoints/bodyPoints above.
func recoveryFromScore(score int) RecoveryCapacity {
	switch {
	case score >= 3:
		return RecoveryHigh
	case score >= 1:
		return RecoveryModerate
	default:
		return RecoveryLow
	}
}

// lifeBandwidth — design doc §5.2: direct mapping from Demand, no
// summing. "Packed" and "Unpredictable" both land in Low — see the
// design doc's own note in §5.2 that an earlier "predictability"
// question was folded into this one for that exact reason.
func lifeBandwidth(a DemandAnswer) (LifeBandwidth, error) {
	switch a {
	case DemandLightAndOpen:
		return BandwidthHigh, nil
	case DemandNormal:
		return BandwidthModerate, nil
	case DemandPacked:
		return BandwidthLow, nil
	case DemandUnpredictable:
		return BandwidthLow, nil
	default:
		return "", fmt.Errorf("checkin: invalid demand answer %q", a)
	}
}

// buildReadiness — design doc §5.3: direct mapping from Need.
func buildReadiness(a NeedAnswer) (BuildReadiness, error) {
	switch a {
	case NeedPushMe:
		return BuildPushForward, nil
	case NeedMeetMeWhereImAt:
		return BuildMaintain, nil
	case NeedLetMeReset:
		return BuildPullBack, nil
	default:
		return "", fmt.Errorf("checkin: invalid need answer %q", a)
	}
}
