package recovery_test

import (
	"testing"

	"viv/internal/core/activity"
	"viv/internal/core/domain"
	"viv/internal/core/recovery"
)

func TestCategorizeSession(t *testing.T) {
	tests := []struct {
		name      string
		activity  activity.ID
		intensity activity.IntensityLevel
		want      recovery.EffectCategory
	}{
		{"strength H is strength-neural", activity.Strength, activity.IntensityH, recovery.EffectStrengthNeural},
		{"strength M is strength-neural (hypertrophy)", activity.Strength, activity.IntensityM, recovery.EffectStrengthNeural},
		{"running L is aerobic", activity.Running, activity.IntensityL, recovery.EffectAerobic},
		{"running H is anaerobic", activity.Running, activity.IntensityH, recovery.EffectAnaerobic},
		{"HIIT is mixed (concurrent aerobic+anaerobic)", activity.HIIT, activity.IntensityH, recovery.EffectMixedHybrid},
		{"functional M is mixed (concurrent strength+aerobic)", activity.Functional, activity.IntensityM, recovery.EffectMixedHybrid},
		{"yoga AR is mobility (recovery effect)", activity.Yoga, activity.IntensityAR, recovery.EffectMobility},
		{"mobility AR stays mobility even though it's 2 concurrent effects", activity.Mobility, activity.IntensityAR, recovery.EffectMobility},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := recovery.CategorizeSession(tt.activity, tt.intensity)
			if !ok {
				t.Fatalf("CategorizeSession(%s, %s) not ok", tt.activity, tt.intensity)
			}
			if got != tt.want {
				t.Errorf("CategorizeSession(%s, %s) = %s, want %s", tt.activity, tt.intensity, got, tt.want)
			}
		})
	}
}

func TestCategorizeSession_UnknownCombo(t *testing.T) {
	// Pilates has no H intensity in its IntensityRange.
	_, ok := recovery.CategorizeSession(activity.Pilates, activity.IntensityH)
	if ok {
		t.Error("expected ok=false for an intensity Pilates doesn't define")
	}
}

func TestDeriveCostTier_BaseOnly(t *testing.T) {
	// Strength@L has RecoveryCost=1 in the taxonomy -> Low, no modifiers.
	tier, ok := recovery.DeriveCostTier(recovery.SessionInputs{
		ActivityType: activity.Strength, Intensity: activity.IntensityL,
		MuscleGroup: activity.MuscleGroupUpper, DurationMinutes: 30,
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if tier != domain.RecoveryCostLow {
		t.Errorf("tier = %s, want low", tier)
	}
}

func TestDeriveCostTier_FullBodyBumpsOneTier(t *testing.T) {
	// Strength@L base=1 (low); full_body should bump to medium.
	tier, ok := recovery.DeriveCostTier(recovery.SessionInputs{
		ActivityType: activity.Strength, Intensity: activity.IntensityL,
		MuscleGroup: activity.MuscleGroupFullBody, DurationMinutes: 30,
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if tier != domain.RecoveryCostMedium {
		t.Errorf("tier = %s, want medium", tier)
	}
}

func TestDeriveCostTier_LongDurationBumpsOneTier(t *testing.T) {
	tier, ok := recovery.DeriveCostTier(recovery.SessionInputs{
		ActivityType: activity.Strength, Intensity: activity.IntensityL,
		MuscleGroup: activity.MuscleGroupUpper, DurationMinutes: 60,
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if tier != domain.RecoveryCostMedium {
		t.Errorf("tier = %s, want medium", tier)
	}
}

func TestDeriveCostTier_FullBodyAndLongDurationDoNotStack(t *testing.T) {
	// Both modifiers true at once should still only bump ONE tier, not two.
	tier, ok := recovery.DeriveCostTier(recovery.SessionInputs{
		ActivityType: activity.Strength, Intensity: activity.IntensityL,
		MuscleGroup: activity.MuscleGroupFullBody, DurationMinutes: 60,
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if tier != domain.RecoveryCostMedium {
		t.Errorf("tier = %s, want medium (not high — modifiers shouldn't stack)", tier)
	}
}

func TestDeriveCostTier_CapsAtHigh(t *testing.T) {
	// Strength@H base=3 (already high); a bump must not overflow past high.
	tier, ok := recovery.DeriveCostTier(recovery.SessionInputs{
		ActivityType: activity.Strength, Intensity: activity.IntensityH,
		MuscleGroup: activity.MuscleGroupFullBody, DurationMinutes: 90,
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if tier != domain.RecoveryCostHigh {
		t.Errorf("tier = %s, want high", tier)
	}
}

func TestBumpTierForSleepDebt(t *testing.T) {
	tests := []struct {
		in   domain.RecoveryCostTier
		want domain.RecoveryCostTier
	}{
		{domain.RecoveryCostLow, domain.RecoveryCostMedium},
		{domain.RecoveryCostMedium, domain.RecoveryCostHigh},
		{domain.RecoveryCostHigh, domain.RecoveryCostHigh},
	}
	for _, tt := range tests {
		if got := recovery.BumpTierForSleepDebt(tt.in); got != tt.want {
			t.Errorf("BumpTierForSleepDebt(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestCopyFor_MobilityIgnoresTier(t *testing.T) {
	low, ok := recovery.CopyFor(recovery.EffectMobility, domain.RecoveryCostLow)
	if !ok {
		t.Fatal("expected ok=true for mobility/low")
	}
	high, ok := recovery.CopyFor(recovery.EffectMobility, domain.RecoveryCostHigh)
	if !ok {
		t.Fatal("expected ok=true for mobility/high")
	}
	if low.Primary.Title != high.Primary.Title {
		t.Error("expected mobility copy to be identical regardless of tier")
	}
}

func TestCopyFor_EveryNonMobilityCategoryHasAllThreeTiers(t *testing.T) {
	cats := []recovery.EffectCategory{
		recovery.EffectAerobic, recovery.EffectAnaerobic,
		recovery.EffectStrengthNeural, recovery.EffectMixedHybrid,
	}
	tiers := []domain.RecoveryCostTier{domain.RecoveryCostLow, domain.RecoveryCostMedium, domain.RecoveryCostHigh}
	for _, c := range cats {
		for _, tier := range tiers {
			copy, ok := recovery.CopyFor(c, tier)
			if !ok {
				t.Errorf("CopyFor(%s, %s) not ok", c, tier)
				continue
			}
			if copy.Primary.Title == "" {
				t.Errorf("CopyFor(%s, %s) has an empty primary title", c, tier)
			}
		}
	}
}
