package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

type fakeInactivityLister struct {
	candidates []*domain.User
	err        error
}

func (f *fakeInactivityLister) ListInactiveSince(_ context.Context, _ time.Time) ([]*domain.User, error) {
	return f.candidates, f.err
}

type fakeContactLookup struct {
	contacts map[string][2]string // userID -> [email, name]
	err      error
}

func (f *fakeContactLookup) GetContact(_ context.Context, userID string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	c, ok := f.contacts[userID]
	if !ok {
		return "", "", errors.New("not found")
	}
	return c[0], c[1], nil
}

type fakeEmailSender struct {
	sentTo  []string
	failFor map[string]bool
}

func (f *fakeEmailSender) SendReEngagementEmail(_ context.Context, email, _ string) error {
	if f.failFor != nil && f.failFor[email] {
		return errors.New("send failed")
	}
	f.sentTo = append(f.sentTo, email)
	return nil
}

func TestRunInactivitySweep_NotifiesAndSchedulesDeletion(t *testing.T) {
	lister := &fakeInactivityLister{candidates: []*domain.User{{ID: "u1"}}}
	contacts := &fakeContactLookup{contacts: map[string][2]string{"u1": {"u1@example.com", "Ana"}}}
	sender := &fakeEmailSender{}
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}

	uc := usecase.NewRunInactivitySweepUseCase(lister, contacts, sender, users)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.sentTo) != 1 || sender.sentTo[0] != "u1@example.com" {
		t.Fatalf("expected email sent to u1@example.com, got %v", sender.sentTo)
	}

	saved := users.users["u1"]
	if saved.PendingDeletionAt == nil {
		t.Fatal("expected PendingDeletionAt to be set after a successful send")
	}
	if saved.PendingDeletionReason != domain.PendingDeletionInactivity {
		t.Fatalf("expected reason=inactivity, got %q", saved.PendingDeletionReason)
	}
}

func TestRunInactivitySweep_SkipsUsersAlreadyPending(t *testing.T) {
	future := time.Now().UTC().Add(10 * 24 * time.Hour)
	lister := &fakeInactivityLister{candidates: []*domain.User{
		{ID: "u1", PendingDeletionAt: &future, PendingDeletionReason: domain.PendingDeletionUserRequested},
	}}
	contacts := &fakeContactLookup{}
	sender := &fakeEmailSender{}
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}

	uc := usecase.NewRunInactivitySweepUseCase(lister, contacts, sender, users)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.sentTo) != 0 {
		t.Fatalf("expected no email sent for an already-pending user, got %v", sender.sentTo)
	}
}

func TestRunInactivitySweep_EmailFailureDoesNotScheduleDeletion(t *testing.T) {
	lister := &fakeInactivityLister{candidates: []*domain.User{{ID: "u1"}}}
	contacts := &fakeContactLookup{contacts: map[string][2]string{"u1": {"u1@example.com", "Ana"}}}
	sender := &fakeEmailSender{failFor: map[string]bool{"u1@example.com": true}}
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}

	uc := usecase.NewRunInactivitySweepUseCase(lister, contacts, sender, users)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if users.users["u1"].PendingDeletionAt != nil {
		t.Fatal("expected no deletion scheduled when the email send fails")
	}
}

func TestRunInactivitySweep_ContactLookupFailureSkipsUser(t *testing.T) {
	lister := &fakeInactivityLister{candidates: []*domain.User{{ID: "u1"}}}
	contacts := &fakeContactLookup{err: errors.New("firebase down")}
	sender := &fakeEmailSender{}
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}

	uc := usecase.NewRunInactivitySweepUseCase(lister, contacts, sender, users)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.sentTo) != 0 {
		t.Fatal("expected no email attempted when contact lookup fails")
	}
}

func TestRunInactivitySweep_ListerErrorPropagates(t *testing.T) {
	lister := &fakeInactivityLister{err: errors.New("boom")}
	uc := usecase.NewRunInactivitySweepUseCase(lister, &fakeContactLookup{}, &fakeEmailSender{}, &fakeUserRepo{users: map[string]*domain.User{}})

	if err := uc.Execute(context.Background()); err == nil {
		t.Fatal("expected lister error to propagate")
	}
}
