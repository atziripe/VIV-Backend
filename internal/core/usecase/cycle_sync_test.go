package usecase

import (
	"testing"

	"viv/internal/core/domain"
)

// TestDaysUntilNextPhase_MatchesPhaseForDay is a regression test for a bug
// where DaysUntilNextPhase's switch cases ("early_follicular",
// "late_follicular", "ovulation") never matched phaseForDay's actual return
// values ("follicular", "ovulatory"), silently falling through to the
// default case (always 1) for any user in those phases.
func TestDaysUntilNextPhase_MatchesPhaseForDay(t *testing.T) {
	const duration = 28 // ovulationDay = 14
	const period = 5

	tests := []struct {
		name      string
		day       int
		wantPhase string
		wantDays  int
	}{
		{"menstrual, day 1", 1, "menstrual", 5},
		{"menstrual, last day", 5, "menstrual", 1},
		{"follicular, first day", 6, "follicular", 7}, // ends at day 12 (ovulationDay-2)
		{"follicular, last day", 12, "follicular", 1},
		{"ovulatory, first day", 13, "ovulatory", 3}, // ends at day 15 (ovulationDay+1)
		{"ovulatory, last day", 15, "ovulatory", 1},
		{"early_luteal, first day", 16, "early_luteal", 4}, // ends at day 19 (ovulationDay+5)
		{"early_luteal, last day", 19, "early_luteal", 1},
		{"late_luteal, first day", 20, "late_luteal", 9}, // ends at day 28 (duration)
		{"late_luteal, last day", 28, "late_luteal", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPhase := phaseForDay(tt.day, duration, period)
			if gotPhase != tt.wantPhase {
				t.Fatalf("phaseForDay(%d, %d, %d) = %q, want %q", tt.day, duration, period, gotPhase, tt.wantPhase)
			}

			user := &domain.User{CycleDay: tt.day, CycleDuration: "28", PeriodDuration: "5"}
			gotDays := DaysUntilNextPhase(user)
			if gotDays != tt.wantDays {
				t.Errorf("DaysUntilNextPhase() for day %d (%s) = %d, want %d", tt.day, tt.wantPhase, gotDays, tt.wantDays)
			}
			if gotDays <= 0 {
				t.Errorf("DaysUntilNextPhase() for day %d (%s) = %d, must always be positive", tt.day, tt.wantPhase, gotDays)
			}
		})
	}
}
