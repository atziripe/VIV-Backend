package usecase_test

import (
	"context"
	"testing"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

type fakeUserRepo struct {
	users map[string]*domain.User
}

func (f *fakeUserRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	return f.users[id], nil
}

func (f *fakeUserRepo) Save(_ context.Context, user *domain.User) error {
	f.users[user.ID] = user
	return nil
}

func TestLogPeriodStart_RecalibratesCycleWithoutCheckinGate(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {
			ID:             "u1",
			CycleDuration:  "28",
			PeriodDuration: "5",
			CycleDay:       20, // was deep into late luteal per the old anchor
			CyclePhase:     "late_luteal",
		},
	}}
	uc := usecase.NewLogPeriodStartUseCase(repo)

	// Report the period starting today — no Date set, defaults to now.
	out, err := uc.Execute(context.Background(), usecase.LogPeriodStartInput{UserID: "u1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if out.CycleDay != 1 {
		t.Errorf("CycleDay = %d, want 1 (today is the new anchor)", out.CycleDay)
	}
	if out.CurrentPhase != string(domain.PhaseMenstrual) {
		t.Errorf("CurrentPhase = %q, want %q", out.CurrentPhase, domain.PhaseMenstrual)
	}

	saved := repo.users["u1"]
	if saved.CycleAnchorAt == nil {
		t.Fatal("expected CycleAnchorAt to be set")
	}
	if saved.CycleDay != 1 {
		t.Errorf("persisted CycleDay = %d, want 1", saved.CycleDay)
	}
}

func TestLogPeriodStart_UnknownUserErrors(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	uc := usecase.NewLogPeriodStartUseCase(repo)

	_, err := uc.Execute(context.Background(), usecase.LogPeriodStartInput{UserID: "ghost"})
	if err == nil {
		t.Fatal("expected an error for an unknown user")
	}
}

func TestLogPeriodStart_BackdatedReport(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5", CycleDay: 1},
	}}
	uc := usecase.NewLogPeriodStartUseCase(repo)

	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	out, err := uc.Execute(context.Background(), usecase.LogPeriodStartInput{UserID: "u1", Date: yesterday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if out.CycleDay != 2 {
		t.Errorf("CycleDay = %d, want 2 (period started yesterday)", out.CycleDay)
	}
}
