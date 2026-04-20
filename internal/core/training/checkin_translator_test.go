package training_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/training"
)

// ============================================================================
// RECOVERY CAPACITY
// ============================================================================

func TestTranslate_Recovery_High(t *testing.T) {
	// Consistent sleep + energised body = 2+2 = 4 → high
	checkin := domain.Checkin{
		Sleep:          domain.SleepConsistent,
		Body:           domain.BodyEnergised,
		Demand:         domain.DemandManageable,
		Predictability: domain.PredictabilityPredictable,
		LastWeekFeel:   domain.LastWeekBalanced,
		Readiness:      domain.ReadinessBuild,
	}

	dims := training.TranslateCheckin(checkin)

	if dims.Recovery != domain.RecoveryHigh {
		t.Errorf("expected RecoveryHigh, got %q", dims.Recovery)
	}
}

func TestTranslate_Recovery_Moderate(t *testing.T) {
	// Inconsistent sleep + neutral body = 1+1 = 2 → moderate
	checkin := domain.Checkin{
		Sleep:          domain.SleepInconsistent,
		Body:           domain.BodyNeutral,
		Demand:         domain.DemandManageable,
		Predictability: domain.PredictabilityPredictable,
		LastWeekFeel:   domain.LastWeekBalanced,
		Readiness:      domain.ReadinessBuild,
	}

	dims := training.TranslateCheckin(checkin)

	if dims.Recovery != domain.RecoveryModerate {
		t.Errorf("expected RecoveryModerate, got %q", dims.Recovery)
	}
}

func TestTranslate_Recovery_Low(t *testing.T) {
	// Poor sleep + inflamed body = -1 + -1 = -2 → low
	checkin := domain.Checkin{
		Sleep:          domain.SleepPoor,
		Body:           domain.BodyInflamed,
		Demand:         domain.DemandManageable,
		Predictability: domain.PredictabilityPredictable,
		LastWeekFeel:   domain.LastWeekBalanced,
		Readiness:      domain.ReadinessBuild,
	}

	dims := training.TranslateCheckin(checkin)

	if dims.Recovery != domain.RecoveryLow {
		t.Errorf("expected RecoveryLow, got %q", dims.Recovery)
	}
}

// ============================================================================
// LIFE BANDWIDTH
// ============================================================================

func TestTranslate_Bandwidth_High(t *testing.T) {
	// Light demand + predictable = 2+2 = 4 → high
	checkin := domain.Checkin{
		Sleep:          domain.SleepConsistent,
		Body:           domain.BodyEnergised,
		Demand:         domain.DemandLight,
		Predictability: domain.PredictabilityPredictable,
		LastWeekFeel:   domain.LastWeekBalanced,
		Readiness:      domain.ReadinessBuild,
	}

	dims := training.TranslateCheckin(checkin)

	if dims.Bandwidth != domain.BandwidthHigh {
		t.Errorf("expected BandwidthHigh, got %q", dims.Bandwidth)
	}
}

func TestTranslate_Bandwidth_Low(t *testing.T) {
	// Overloaded + chaotic = -1 + -1 = -2 → low
	checkin := domain.Checkin{
		Sleep:          domain.SleepConsistent,
		Body:           domain.BodyEnergised,
		Demand:         domain.DemandOverloaded,
		Predictability: domain.PredictabilityChaotic,
		LastWeekFeel:   domain.LastWeekBalanced,
		Readiness:      domain.ReadinessBuild,
	}

	dims := training.TranslateCheckin(checkin)

	if dims.Bandwidth != domain.BandwidthLow {
		t.Errorf("expected BandwidthLow, got %q", dims.Bandwidth)
	}
}

// ============================================================================
// BUILD READINESS
// ============================================================================

func TestTranslate_Build_PushForward(t *testing.T) {
	// Too light + ready to build = 1+1 = 2 → push_forward
	checkin := domain.Checkin{
		Sleep:          domain.SleepConsistent,
		Body:           domain.BodyEnergised,
		Demand:         domain.DemandManageable,
		Predictability: domain.PredictabilityPredictable,
		LastWeekFeel:   domain.LastWeekTooLight,
		Readiness:      domain.ReadinessBuild,
	}

	dims := training.TranslateCheckin(checkin)

	if dims.Build != domain.ReadinessPushForward {
		t.Errorf("expected ReadinessPushForward, got %q", dims.Build)
	}
}

func TestTranslate_Build_Maintain(t *testing.T) {
	// Balanced + ready to build = 0+1 = 1 → push_forward
	// Balanced + fragile = 0 + -1 = -1 → pull_back
	// Too light + fragile = 1 + -1 = 0 → maintain
	checkin := domain.Checkin{
		Sleep:          domain.SleepConsistent,
		Body:           domain.BodyEnergised,
		Demand:         domain.DemandManageable,
		Predictability: domain.PredictabilityPredictable,
		LastWeekFeel:   domain.LastWeekTooLight,
		Readiness:      domain.ReadinessFragile,
	}

	dims := training.TranslateCheckin(checkin)

	if dims.Build != domain.ReadinessMaintain {
		t.Errorf("expected ReadinessMaintain, got %q", dims.Build)
	}
}

func TestTranslate_Build_PullBack(t *testing.T) {
	// Too much + stabilize = -1 + -2 = -3 → pull_back
	checkin := domain.Checkin{
		Sleep:          domain.SleepConsistent,
		Body:           domain.BodyEnergised,
		Demand:         domain.DemandManageable,
		Predictability: domain.PredictabilityPredictable,
		LastWeekFeel:   domain.LastWeekTooMuch,
		Readiness:      domain.ReadinessStabilize,
	}

	dims := training.TranslateCheckin(checkin)

	if dims.Build != domain.ReadinessPullBack {
		t.Errorf("expected ReadinessPullBack, got %q", dims.Build)
	}
}

// ============================================================================
// REST WEEK TRIGGER
// ============================================================================

func TestRestWeek_Recommended_TwoOfThree(t *testing.T) {
	// Recovery low + Bandwidth low + Build maintain → 2 of 3 → recommended
	dims := domain.CheckinDimensions{
		Recovery:  domain.RecoveryLow,
		Bandwidth: domain.BandwidthLow,
		Build:     domain.ReadinessMaintain,
	}

	if !dims.RestWeekRecommended() {
		t.Error("expected rest week to be recommended with 2 low dimensions")
	}
}

func TestRestWeek_Recommended_AllThree(t *testing.T) {
	dims := domain.CheckinDimensions{
		Recovery:  domain.RecoveryLow,
		Bandwidth: domain.BandwidthLow,
		Build:     domain.ReadinessPullBack,
	}

	if !dims.RestWeekRecommended() {
		t.Error("expected rest week to be recommended with all 3 low dimensions")
	}
}

func TestRestWeek_NotRecommended_OneOfThree(t *testing.T) {
	// Only recovery is low → not enough
	dims := domain.CheckinDimensions{
		Recovery:  domain.RecoveryLow,
		Bandwidth: domain.BandwidthModerate,
		Build:     domain.ReadinessMaintain,
	}

	if dims.RestWeekRecommended() {
		t.Error("should not recommend rest week with only 1 low dimension")
	}
}

func TestRestWeek_NotRecommended_AllGood(t *testing.T) {
	dims := domain.CheckinDimensions{
		Recovery:  domain.RecoveryHigh,
		Bandwidth: domain.BandwidthHigh,
		Build:     domain.ReadinessPushForward,
	}

	if dims.RestWeekRecommended() {
		t.Error("should not recommend rest week when all dimensions are good")
	}
}

// ============================================================================
// INTEGRATION — full check-in → dimensions → rest week decision
// ============================================================================

func TestTranslate_WorstCase_TriggersRestWeek(t *testing.T) {
	// Worst possible check-in
	checkin := domain.Checkin{
		Sleep:          domain.SleepPoor,
		Body:           domain.BodyInflamed,
		AppetiteV2:     domain.AppetiteAllOver,
		Demand:         domain.DemandOverloaded,
		Predictability: domain.PredictabilityChaotic,
		LastWeekFeel:   domain.LastWeekTooMuch,
		Readiness:      domain.ReadinessStabilize,
	}

	dims := training.TranslateCheckin(checkin)

	if dims.Recovery != domain.RecoveryLow {
		t.Errorf("expected RecoveryLow, got %q", dims.Recovery)
	}
	if dims.Bandwidth != domain.BandwidthLow {
		t.Errorf("expected BandwidthLow, got %q", dims.Bandwidth)
	}
	if dims.Build != domain.ReadinessPullBack {
		t.Errorf("expected ReadinessPullBack, got %q", dims.Build)
	}
	if !dims.RestWeekRecommended() {
		t.Error("worst case check-in should trigger rest week recommendation")
	}
}

func TestTranslate_BestCase_NoPullBack(t *testing.T) {
	// Best possible check-in
	checkin := domain.Checkin{
		Sleep:          domain.SleepConsistent,
		Body:           domain.BodyEnergised,
		AppetiteV2:     domain.AppetiteStable,
		Demand:         domain.DemandLight,
		Predictability: domain.PredictabilityPredictable,
		LastWeekFeel:   domain.LastWeekTooLight,
		Readiness:      domain.ReadinessBuild,
	}

	dims := training.TranslateCheckin(checkin)

	if dims.Recovery != domain.RecoveryHigh {
		t.Errorf("expected RecoveryHigh, got %q", dims.Recovery)
	}
	if dims.Bandwidth != domain.BandwidthHigh {
		t.Errorf("expected BandwidthHigh, got %q", dims.Bandwidth)
	}
	if dims.Build != domain.ReadinessPushForward {
		t.Errorf("expected ReadinessPushForward, got %q", dims.Build)
	}
	if dims.RestWeekRecommended() {
		t.Error("best case check-in should not trigger rest week")
	}
}
