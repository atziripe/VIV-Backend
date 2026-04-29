package recovery_test

import (
	"testing"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/recovery"
)

func TestSelectBanner_FirstWeek(t *testing.T) {
	input := recovery.BannerInput{
		Phase:       domain.PhaseFollicular,
		IsFirstWeek: true,
		Weekday:     time.Wednesday,
	}

	banner := recovery.SelectBanner(input, nil)

	if banner.Variant != "O" {
		t.Errorf("expected variant O, got %s", banner.Variant)
	}
	if banner.Title == "" {
		t.Error("expected non-empty title")
	}
}

func TestSelectBanner_TrainingDay(t *testing.T) {
	input := recovery.BannerInput{
		Phase:         domain.PhaseFollicular,
		IsTrainingDay: true,
		Weekday:       time.Monday,
	}

	banner := recovery.SelectBanner(input, nil)

	if banner.Variant != "T" {
		t.Errorf("expected variant T, got %s", banner.Variant)
	}
	if banner.Label != "Today — Monday · training day" {
		t.Errorf("unexpected label: %s", banner.Label)
	}
}

func TestSelectBanner_RestAfterSession(t *testing.T) {
	input := recovery.BannerInput{
		Phase:               domain.PhaseLateLuteal,
		IsTrainingDay:       false,
		TrainedYesterday:    true,
		YesterdayModality:   domain.ModalityStrength,
		ConsecutiveRestDays: 1,
		Weekday:             time.Tuesday,
	}

	banner := recovery.SelectBanner(input, nil)

	if banner.Variant != "R" {
		t.Errorf("expected variant R, got %s", banner.Variant)
	}
	if banner.Title != "Yesterday's strength session is converting to strength right now." {
		t.Errorf("unexpected title: %s", banner.Title)
	}
}

func TestSelectBanner_RestAfterPilates_AdaptationOutcome(t *testing.T) {
	input := recovery.BannerInput{
		Phase:               domain.PhaseFollicular,
		IsTrainingDay:       false,
		TrainedYesterday:    true,
		YesterdayModality:   domain.ModalityPilates,
		ConsecutiveRestDays: 1,
		Weekday:             time.Thursday,
	}

	banner := recovery.SelectBanner(input, nil)

	if banner.Title != "Yesterday's Pilates session is converting to adaptation right now." {
		t.Errorf("unexpected title: %s", banner.Title)
	}
}

func TestSelectBanner_ConsecutiveRest(t *testing.T) {
	input := recovery.BannerInput{
		Phase:               domain.PhaseEarlyLuteal,
		IsTrainingDay:       false,
		TrainedYesterday:    false,
		ConsecutiveRestDays: 3,
		Weekday:             time.Saturday,
	}

	banner := recovery.SelectBanner(input, nil)

	if banner.Variant != "N" {
		t.Errorf("expected variant N, got %s", banner.Variant)
	}
	if banner.Title != "Your body is in active reset." {
		t.Errorf("unexpected title: %s", banner.Title)
	}
}

func TestSelectBanner_TrainingDayOverridesEverything(t *testing.T) {
	// Even if trained yesterday, training day variant wins
	input := recovery.BannerInput{
		Phase:            domain.PhaseFollicular,
		IsTrainingDay:    true,
		TrainedYesterday: true,
		Weekday:          time.Wednesday,
	}

	banner := recovery.SelectBanner(input, nil)

	if banner.Variant != "T" {
		t.Errorf("expected variant T (training day overrides), got %s", banner.Variant)
	}
}

func TestSelectBanner_FirstWeekOverridesEverything(t *testing.T) {
	input := recovery.BannerInput{
		Phase:         domain.PhaseFollicular,
		IsTrainingDay: true,
		IsFirstWeek:   true,
		Weekday:       time.Monday,
	}

	banner := recovery.SelectBanner(input, nil)

	if banner.Variant != "O" {
		t.Errorf("expected variant O (first week overrides), got %s", banner.Variant)
	}
}

func TestSelectBanner_AllPhases_RestAfterStrength(t *testing.T) {
	phases := []domain.CyclePhase{
		domain.PhaseMenstrual, domain.PhaseFollicular, domain.PhaseOvulatory,
		domain.PhaseEarlyLuteal, domain.PhaseLateLuteal,
	}

	for _, phase := range phases {
		input := recovery.BannerInput{
			Phase:               phase,
			IsTrainingDay:       false,
			TrainedYesterday:    true,
			YesterdayModality:   domain.ModalityStrength,
			ConsecutiveRestDays: 1,
			Weekday:             time.Tuesday,
		}

		banner := recovery.SelectBanner(input, nil)

		if banner.Variant != "R" {
			t.Errorf("phase %s: expected variant R, got %s", phase, banner.Variant)
		}
	}
}
