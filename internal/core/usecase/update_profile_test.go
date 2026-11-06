package usecase_test

import (
	"context"
	"errors"
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

// newTestUpdateProfileUseCase builds an UpdateProfileUseCase with every
// optional collaborator left nil — sufficient for the plain field-edit and
// validation behavior these tests cover, none of which touches the
// nutrition-recompute or plan-regen side effects (both already gated on
// user.LastActivePlanID being set, which none of these fixtures set).
func newTestUpdateProfileUseCase(repo *fakeUserRepo) *usecase.UpdateProfileUseCase {
	return usecase.NewUpdateProfileUseCase(repo, nil, nil, nil, nil, nil, nil)
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func TestUpdateProfile_ValidCycleDurationIsAccepted(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5"},
	}}
	uc := newTestUpdateProfileUseCase(repo)

	_, err := uc.Execute(context.Background(), usecase.UpdateProfileInput{
		UserID:        "u1",
		CycleDuration: strPtr("33"),
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if repo.users["u1"].CycleDuration != "33" {
		t.Errorf("CycleDuration = %q, want %q", repo.users["u1"].CycleDuration, "33")
	}
}

func TestUpdateProfile_BucketedCycleDurationStillAccepted(t *testing.T) {
	// The onboarding-style bucketed strings must keep working — this
	// validation is additive, not a replacement for normalizeCycleDuration.
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5"},
	}}
	uc := newTestUpdateProfileUseCase(repo)

	_, err := uc.Execute(context.Background(), usecase.UpdateProfileInput{
		UserID:        "u1",
		CycleDuration: strPtr("31-35 days"),
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if repo.users["u1"].CycleDuration != "33" {
		t.Errorf("CycleDuration = %q, want %q (normalized from the bucket)", repo.users["u1"].CycleDuration, "33")
	}
}

func TestUpdateProfile_OutOfRangeCycleDurationIsRejected(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5"},
	}}
	uc := newTestUpdateProfileUseCase(repo)

	_, err := uc.Execute(context.Background(), usecase.UpdateProfileInput{
		UserID:        "u1",
		CycleDuration: strPtr("4"),
	})
	var invalid usecase.InvalidCycleDurationError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected InvalidCycleDurationError, got %v", err)
	}
	if repo.users["u1"].CycleDuration != "28" {
		t.Errorf("CycleDuration = %q, want %q (rejected edit must not persist)", repo.users["u1"].CycleDuration, "28")
	}
}

func TestUpdateProfile_NonNumericCycleDurationIsRejected(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5"},
	}}
	uc := newTestUpdateProfileUseCase(repo)

	_, err := uc.Execute(context.Background(), usecase.UpdateProfileInput{
		UserID:        "u1",
		CycleDuration: strPtr("banana"),
	})
	var invalid usecase.InvalidCycleDurationError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected InvalidCycleDurationError, got %v", err)
	}
}

func TestUpdateProfile_RejectedCycleDurationDoesNotPersistOtherFields(t *testing.T) {
	// A single PATCH can carry several fields at once — an invalid
	// cycle_duration must fail the whole edit, not silently save the rest.
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", Name: "Ana", CycleDuration: "28", PeriodDuration: "5"},
	}}
	uc := newTestUpdateProfileUseCase(repo)

	_, err := uc.Execute(context.Background(), usecase.UpdateProfileInput{
		UserID:        "u1",
		Name:          strPtr("Beatriz"),
		CycleDuration: strPtr("999"),
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if repo.users["u1"].Name != "Ana" {
		t.Errorf("Name = %q, want %q (nothing should persist when validation fails)", repo.users["u1"].Name, "Ana")
	}
}

func TestUpdateProfile_SetsCycleEstimationDisabled(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5"},
	}}
	uc := newTestUpdateProfileUseCase(repo)

	_, err := uc.Execute(context.Background(), usecase.UpdateProfileInput{
		UserID:                  "u1",
		CycleEstimationDisabled: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !repo.users["u1"].CycleEstimationDisabled {
		t.Error("expected CycleEstimationDisabled to be true")
	}
}

func TestUpdateProfile_OmittedCycleEstimationDisabledLeavesItUnchanged(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5", CycleEstimationDisabled: true},
	}}
	uc := newTestUpdateProfileUseCase(repo)

	_, err := uc.Execute(context.Background(), usecase.UpdateProfileInput{
		UserID: "u1",
		Name:   strPtr("Beatriz"),
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !repo.users["u1"].CycleEstimationDisabled {
		t.Error("expected CycleEstimationDisabled to stay true — field wasn't sent, so it shouldn't change")
	}
}

func TestUpdateProfile_UnknownUserErrors(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	uc := newTestUpdateProfileUseCase(repo)

	_, err := uc.Execute(context.Background(), usecase.UpdateProfileInput{UserID: "ghost"})
	if err == nil {
		t.Fatal("expected an error for an unknown user")
	}
}
