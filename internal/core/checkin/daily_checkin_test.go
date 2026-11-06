package checkin_test

import (
	"testing"

	"viv/internal/core/checkin"
)

// TestRecoveryCapacity_AllSixteenSleepBodyCombinations covers every
// Sleep×Body pair explicitly — the expected bucket is hand-computed from
// the design doc's point table, not re-derived from the implementation,
// so this actually catches a wrong point value or a wrong bucket cutoff.
func TestRecoveryCapacity_AllSixteenSleepBodyCombinations(t *testing.T) {
	tests := []struct {
		sleep checkin.SleepAnswer
		body  checkin.BodyAnswer
		want  checkin.RecoveryCapacity
	}{
		// Sleep = deep_and_restful (+2)
		{checkin.SleepDeepAndRestful, checkin.BodyStrongAndResponsive, checkin.RecoveryHigh},     // 2+2=4
		{checkin.SleepDeepAndRestful, checkin.BodyNormal, checkin.RecoveryHigh},                  // 2+1=3
		{checkin.SleepDeepAndRestful, checkin.BodyHeavierThanUsual, checkin.RecoveryModerate},    // 2+0=2
		{checkin.SleepDeepAndRestful, checkin.BodySensitiveOrReactive, checkin.RecoveryModerate}, // 2-1=1

		// Sleep = normal (+1)
		{checkin.SleepNormal, checkin.BodyStrongAndResponsive, checkin.RecoveryHigh},  // 1+2=3
		{checkin.SleepNormal, checkin.BodyNormal, checkin.RecoveryModerate},           // 1+1=2
		{checkin.SleepNormal, checkin.BodyHeavierThanUsual, checkin.RecoveryModerate}, // 1+0=1
		{checkin.SleepNormal, checkin.BodySensitiveOrReactive, checkin.RecoveryLow},   // 1-1=0

		// Sleep = restless (0)
		{checkin.SleepRestless, checkin.BodyStrongAndResponsive, checkin.RecoveryModerate}, // 0+2=2
		{checkin.SleepRestless, checkin.BodyNormal, checkin.RecoveryModerate},              // 0+1=1
		{checkin.SleepRestless, checkin.BodyHeavierThanUsual, checkin.RecoveryLow},         // 0+0=0
		{checkin.SleepRestless, checkin.BodySensitiveOrReactive, checkin.RecoveryLow},      // 0-1=-1

		// Sleep = barely_slept (-1)
		{checkin.SleepBarelySlept, checkin.BodyStrongAndResponsive, checkin.RecoveryModerate}, // -1+2=1
		{checkin.SleepBarelySlept, checkin.BodyNormal, checkin.RecoveryLow},                   // -1+1=0
		{checkin.SleepBarelySlept, checkin.BodyHeavierThanUsual, checkin.RecoveryLow},         // -1+0=-1
		{checkin.SleepBarelySlept, checkin.BodySensitiveOrReactive, checkin.RecoveryLow},      // -1-1=-2
	}

	if len(tests) != 16 {
		t.Fatalf("test table has %d entries, want all 16 Sleep×Body combinations", len(tests))
	}

	for _, tt := range tests {
		dims, err := checkin.Derive(checkin.DailyCheckin{
			Sleep:  tt.sleep,
			Body:   tt.body,
			Demand: checkin.DemandNormal, // fixed, irrelevant to RecoveryCapacity
			Need:   checkin.NeedMeetMeWhereImAt,
		})
		if err != nil {
			t.Errorf("Derive(sleep=%s, body=%s) returned error: %v", tt.sleep, tt.body, err)
			continue
		}
		if dims.RecoveryCapacity != tt.want {
			t.Errorf("Derive(sleep=%s, body=%s).RecoveryCapacity = %s, want %s",
				tt.sleep, tt.body, dims.RecoveryCapacity, tt.want)
		}
	}
}

func TestLifeBandwidth_EveryDemandValue(t *testing.T) {
	tests := []struct {
		demand checkin.DemandAnswer
		want   checkin.LifeBandwidth
	}{
		{checkin.DemandLightAndOpen, checkin.BandwidthHigh},
		{checkin.DemandNormal, checkin.BandwidthModerate},
		{checkin.DemandPacked, checkin.BandwidthLow},
		{checkin.DemandUnpredictable, checkin.BandwidthLow},
	}

	for _, tt := range tests {
		dims, err := checkin.Derive(checkin.DailyCheckin{
			Sleep:  checkin.SleepNormal,
			Body:   checkin.BodyNormal,
			Demand: tt.demand,
			Need:   checkin.NeedMeetMeWhereImAt,
		})
		if err != nil {
			t.Errorf("Derive(demand=%s) returned error: %v", tt.demand, err)
			continue
		}
		if dims.LifeBandwidth != tt.want {
			t.Errorf("Derive(demand=%s).LifeBandwidth = %s, want %s", tt.demand, dims.LifeBandwidth, tt.want)
		}
	}
}

func TestBuildReadiness_EveryNeedValue(t *testing.T) {
	tests := []struct {
		need checkin.NeedAnswer
		want checkin.BuildReadiness
	}{
		{checkin.NeedPushMe, checkin.BuildPushForward},
		{checkin.NeedMeetMeWhereImAt, checkin.BuildMaintain},
		{checkin.NeedLetMeReset, checkin.BuildPullBack},
	}

	for _, tt := range tests {
		dims, err := checkin.Derive(checkin.DailyCheckin{
			Sleep:  checkin.SleepNormal,
			Body:   checkin.BodyNormal,
			Demand: checkin.DemandNormal,
			Need:   tt.need,
		})
		if err != nil {
			t.Errorf("Derive(need=%s) returned error: %v", tt.need, err)
			continue
		}
		if dims.BuildReadiness != tt.want {
			t.Errorf("Derive(need=%s).BuildReadiness = %s, want %s", tt.need, dims.BuildReadiness, tt.want)
		}
	}
}

// TestInvalidAnswersReturnExplicitErrors: a malformed check-in answer must
// never silently resolve to a plausible-looking readiness score — every
// field's invalid value must error, and the zero-value ReadinessDimensions
// must be returned alongside it (never a half-filled result).
func TestInvalidAnswersReturnExplicitErrors(t *testing.T) {
	valid := checkin.DailyCheckin{
		Sleep:  checkin.SleepNormal,
		Body:   checkin.BodyNormal,
		Demand: checkin.DemandNormal,
		Need:   checkin.NeedMeetMeWhereImAt,
	}

	tests := []struct {
		name   string
		break_ func(checkin.DailyCheckin) checkin.DailyCheckin
	}{
		{"invalid sleep", func(c checkin.DailyCheckin) checkin.DailyCheckin { c.Sleep = "great"; return c }},
		{"empty sleep", func(c checkin.DailyCheckin) checkin.DailyCheckin { c.Sleep = ""; return c }},
		{"invalid body", func(c checkin.DailyCheckin) checkin.DailyCheckin { c.Body = "fine"; return c }},
		{"invalid demand", func(c checkin.DailyCheckin) checkin.DailyCheckin { c.Demand = "busy"; return c }},
		{"invalid need", func(c checkin.DailyCheckin) checkin.DailyCheckin { c.Need = "surprise_me"; return c }},
		// A legacy-format value from the OLD weekly checkin vocabulary
		// must not be accepted here either — the two systems are
		// deliberately not string-compatible.
		{"old-format sleep value", func(c checkin.DailyCheckin) checkin.DailyCheckin { c.Sleep = "Consistent"; return c }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broken := tt.break_(valid)
			dims, err := checkin.Derive(broken)
			if err == nil {
				t.Fatalf("expected an error, got dims=%+v", dims)
			}
			if dims != (checkin.ReadinessDimensions{}) {
				t.Errorf("expected zero-value ReadinessDimensions alongside the error, got %+v", dims)
			}
		})
	}
}

func TestValidCheckinNeverErrors(t *testing.T) {
	_, err := checkin.Derive(checkin.DailyCheckin{
		Sleep:  checkin.SleepDeepAndRestful,
		Body:   checkin.BodyStrongAndResponsive,
		Demand: checkin.DemandLightAndOpen,
		Need:   checkin.NeedPushMe,
	})
	if err != nil {
		t.Errorf("a fully valid check-in should never error, got: %v", err)
	}
}
