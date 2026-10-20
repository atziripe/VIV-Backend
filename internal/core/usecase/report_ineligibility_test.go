package usecase_test

import (
	"context"
	"errors"
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

type fakeIneligibilityRecorder struct {
	recorded []domain.IneligibilityReason
	err      error
}

func (f *fakeIneligibilityRecorder) RecordIneligibility(_ context.Context, reason domain.IneligibilityReason) error {
	if f.err != nil {
		return f.err
	}
	f.recorded = append(f.recorded, reason)
	return nil
}

type fakeAccountDeleter struct {
	deleted []string
	err     error
}

func (f *fakeAccountDeleter) Execute(_ context.Context, userID string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, userID)
	return nil
}

func TestReportIneligibility_RecordsStatisticAndDeletesAccount(t *testing.T) {
	recorder := &fakeIneligibilityRecorder{}
	deleter := &fakeAccountDeleter{}
	uc := usecase.NewReportIneligibilityUseCase(recorder, deleter)

	err := uc.Execute(context.Background(), "u1", domain.IneligibilityPregnantOrPostpartum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recorder.recorded) != 1 || recorder.recorded[0] != domain.IneligibilityPregnantOrPostpartum {
		t.Fatalf("expected reason recorded, got %v", recorder.recorded)
	}
	if len(deleter.deleted) != 1 || deleter.deleted[0] != "u1" {
		t.Fatalf("expected account u1 deleted, got %v", deleter.deleted)
	}
}

func TestReportIneligibility_InvalidReasonRejected(t *testing.T) {
	recorder := &fakeIneligibilityRecorder{}
	deleter := &fakeAccountDeleter{}
	uc := usecase.NewReportIneligibilityUseCase(recorder, deleter)

	err := uc.Execute(context.Background(), "u1", domain.IneligibilityReason("something_else"))

	var invalidReason usecase.InvalidIneligibilityReasonError
	if !errors.As(err, &invalidReason) {
		t.Fatalf("expected InvalidIneligibilityReasonError, got %v", err)
	}
	if len(deleter.deleted) != 0 {
		t.Fatal("expected no account deletion for an invalid reason")
	}
}

func TestReportIneligibility_StatisticFailureDoesNotBlockDeletion(t *testing.T) {
	recorder := &fakeIneligibilityRecorder{err: errors.New("neon down")}
	deleter := &fakeAccountDeleter{}
	uc := usecase.NewReportIneligibilityUseCase(recorder, deleter)

	err := uc.Execute(context.Background(), "u1", domain.IneligibilityHormonalContraception)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deleter.deleted) != 1 {
		t.Fatal("expected account deletion to proceed despite the statistic failing")
	}
}

func TestReportIneligibility_NilEventsSkipsStatistic(t *testing.T) {
	deleter := &fakeAccountDeleter{}
	uc := usecase.NewReportIneligibilityUseCase(nil, deleter)

	err := uc.Execute(context.Background(), "u1", domain.IneligibilityNotSure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deleter.deleted) != 1 {
		t.Fatal("expected account deletion to proceed with Events unconfigured")
	}
}

func TestReportIneligibility_DeletionFailurePropagates(t *testing.T) {
	recorder := &fakeIneligibilityRecorder{}
	deleter := &fakeAccountDeleter{err: errors.New("firestore down")}
	uc := usecase.NewReportIneligibilityUseCase(recorder, deleter)

	err := uc.Execute(context.Background(), "u1", domain.IneligibilityPerimenopauseOrMenopause)
	if err == nil {
		t.Fatal("expected deletion failure to propagate")
	}
}
