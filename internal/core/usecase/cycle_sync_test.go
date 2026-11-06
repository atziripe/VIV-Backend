package usecase

import (
	"testing"
	"time"

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

// TestSyncUserCycleDaily_FreezesInsteadOfWrappingPastDuration is a
// regression test for the "holding" bug: before this fix, a period that
// never arrived was invisible — the day counter wrapped into a new cycle
// exactly on schedule regardless of whether a real period was ever
// reported, so every downstream phase read (training/nutrition) silently
// acted as if it had happened.
func TestSyncUserCycleDaily_FreezesInsteadOfWrappingPastDuration(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // day 25 of a 28-day cycle
	user := &domain.User{
		CycleDuration:  "28",
		PeriodDuration: "5",
		CycleDay:       25,
		CyclePhase:     "late_luteal",
		CycleUpdatedAt: &anchor,
	}

	// 10 days pass with no period ever reported — well past the assumed
	// 28-day duration.
	now := anchor.AddDate(0, 0, 10)
	if !SyncUserCycleDaily(user, now) {
		t.Fatal("expected SyncUserCycleDaily to report a change")
	}

	if user.CycleDay != 28 {
		t.Errorf("CycleDay = %d, want 28 (frozen at the last day of the assumed cycle, not wrapped)", user.CycleDay)
	}
	if user.CyclePhase != "late_luteal" {
		t.Errorf("CyclePhase = %q, want %q (held, not advanced into a new cycle)", user.CyclePhase, "late_luteal")
	}

	// Syncing again later still doesn't move past day 28 on its own.
	later := now.AddDate(0, 0, 5)
	SyncUserCycleDaily(user, later)
	if user.CycleDay != 28 {
		t.Errorf("CycleDay = %d after a second sync, want still 28 (holding)", user.CycleDay)
	}
}

func TestSyncUserCycleDaily_StillAdvancesNormallyWithinTheCycle(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	user := &domain.User{
		CycleDuration:  "28",
		PeriodDuration: "5",
		CycleDay:       1,
		CyclePhase:     "menstrual",
		CycleUpdatedAt: &anchor,
	}

	SyncUserCycleDaily(user, anchor.AddDate(0, 0, 3))

	if user.CycleDay != 4 {
		t.Errorf("CycleDay = %d, want 4 — normal within-cycle advancement shouldn't be affected by the freeze", user.CycleDay)
	}
}

func TestExpectedPeriodDate(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	user := &domain.User{CycleDuration: "28", CycleAnchorAt: &anchor}

	got := ExpectedPeriodDate(user)
	want := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ExpectedPeriodDate = %v, want %v", got, want)
	}
}

func TestExpectedPeriodDate_NoAnchorYet(t *testing.T) {
	user := &domain.User{CycleDuration: "28"}
	if got := ExpectedPeriodDate(user); !got.IsZero() {
		t.Errorf("ExpectedPeriodDate = %v, want zero time (no anchor to predict from)", got)
	}
}

func TestDaysLate(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) // expected: Sep 29
	user := &domain.User{CycleDuration: "28", CycleAnchorAt: &anchor}

	tests := []struct {
		name string
		now  time.Time
		want int
	}{
		{"exactly on time", time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), 0},
		{"six days late", time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), 6},
		{"four days early", time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC), -4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DaysLate(user, tt.now); got != tt.want {
				t.Errorf("DaysLate() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestApplyCycleStartOverride_RecalibratesDurationFromObservedInterval(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	user := &domain.User{CycleDuration: "28", PeriodDuration: "5", CycleAnchorAt: &anchor}

	// The period actually started 33 days after the last anchor, not 28.
	newStart := anchor.AddDate(0, 0, 33)
	ApplyCycleStartOverride(user, newStart, newStart)

	if user.CycleDuration != "33" {
		t.Errorf("CycleDuration = %q, want %q (recalibrated to the observed interval)", user.CycleDuration, "33")
	}
	if user.CycleDay != 1 {
		t.Errorf("CycleDay = %d, want 1 (today is the new anchor)", user.CycleDay)
	}
}

func TestApplyCycleStartOverride_IgnoresOutOfRangeInterval(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	user := &domain.User{CycleDuration: "28", PeriodDuration: "5", CycleAnchorAt: &anchor}

	// An implausible 4-day interval — a mis-tap, not a real cycle length.
	newStart := anchor.AddDate(0, 0, 4)
	ApplyCycleStartOverride(user, newStart, newStart)

	if user.CycleDuration != "28" {
		t.Errorf("CycleDuration = %q, want %q (kept — 4 days is outside the sane range)", user.CycleDuration, "28")
	}
}

func TestApplyCycleStartOverride_FirstReportNeverRecalibrates(t *testing.T) {
	user := &domain.User{CycleDuration: "28", PeriodDuration: "5"} // no prior anchor

	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ApplyCycleStartOverride(user, start, start)

	if user.CycleDuration != "28" {
		t.Errorf("CycleDuration = %q, want %q (nothing to measure an interval against yet)", user.CycleDuration, "28")
	}
}

func TestExpectedPeriodDate_RespectsCycleEstimationDisabled(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	user := &domain.User{CycleDuration: "28", CycleAnchorAt: &anchor, CycleEstimationDisabled: true}

	if got := ExpectedPeriodDate(user); !got.IsZero() {
		t.Errorf("ExpectedPeriodDate = %v, want zero time (estimation opted out)", got)
	}
	if got := DaysLate(user, anchor.AddDate(0, 0, 40)); got != 0 {
		t.Errorf("DaysLate = %d, want 0 (estimation opted out)", got)
	}
}
