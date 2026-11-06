package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"viv/internal/core/domain"
)

// ============================================================================
// EXPORT USER DATA — the data-portability read (GDPR Art. 20): "give me a
// copy of my data in a usable format." Pure composition over repos that
// already exist, same spirit as GetHomeTimelineUseCase — no new storage, no
// new writes.
//
// Scope: current state per entity (profile, consent status, the active
// nutrition plan, this week's training plan, the legacy plan if still on
// that pipeline), plus full history for every entity that has one —
// lifestyle changes, daily check-ins, session logs, and recovery actions —
// via each repository's ListByUser.
// ============================================================================

type ExportUserDataInput struct {
	UserID string
}

type ExportUserDataOutput struct {
	ExportedAt time.Time

	Profile  *domain.User
	Consents map[domain.ConsentPurpose]domain.Consent

	// LifestyleChanges, DailyCheckins, SessionLogs, and RecoveryActions are
	// each the user's full history, capped at a generous limit (see the
	// export*Limit constants below) rather than the single-date/current
	// state the rest of the product reads.
	LifestyleChanges []*domain.LifestyleChange
	DailyCheckins    []*domain.DailyCheckin
	SessionLogs      []*domain.SessionLog
	RecoveryActions  []*domain.RecoveryAction

	// NutritionPlan is the new pipeline's current standalone plan — nil if
	// the user never generated one (or is still nutrition-pipeline-less).
	NutritionPlan *domain.NutritionPlan

	// CurrentWeekDraft is the new pipeline's training week covering today —
	// nil if none has been generated yet or the current one has rolled over.
	CurrentWeekDraft *WeekDraft

	// LegacyPlan is the old pipeline's most recent domain.Plan (bundled
	// training+nutrition+recovery) — nil for a user who's never touched it.
	LegacyPlan *domain.Plan
}

type ExportUserDataUseCase struct {
	Users            UserRepository
	Consents         ConsentRepository
	LifestyleChanges LifestyleChangeRepository
	DailyCheckins    DailyCheckinRepository
	SessionLogs      SessionLogRepository
	RecoveryActions  RecoveryActionRepository
	NutritionPlans   NutritionPlanRepository
	WeeklyPlanDrafts WeeklyPlanDraftRepository
	Plans            PlanRepository
}

func NewExportUserDataUseCase(
	users UserRepository,
	consents ConsentRepository,
	lifestyleChanges LifestyleChangeRepository,
	dailyCheckins DailyCheckinRepository,
	sessionLogs SessionLogRepository,
	recoveryActions RecoveryActionRepository,
	nutritionPlans NutritionPlanRepository,
	weeklyPlanDrafts WeeklyPlanDraftRepository,
	plans PlanRepository,
) *ExportUserDataUseCase {
	return &ExportUserDataUseCase{
		Users: users, Consents: consents, LifestyleChanges: lifestyleChanges,
		DailyCheckins: dailyCheckins, SessionLogs: sessionLogs, RecoveryActions: recoveryActions,
		NutritionPlans: nutritionPlans, WeeklyPlanDrafts: weeklyPlanDrafts, Plans: plans,
	}
}

// exportLifestyleChangeLimit is generous enough that "full history" is true
// in practice (ListByUser has no unbounded/no-limit mode) without risking
// an unbounded read for a pathological account. Lifestyle changes are
// sporadic events; the daily-record entities below use a larger cap since
// a long-tenured user accrues one per day.
const exportLifestyleChangeLimit = 1000

// exportDailyRecordLimit covers 10+ years of daily records — generous
// enough that "full history" is true in practice for every real account.
const exportDailyRecordLimit = 5000

func (uc *ExportUserDataUseCase) Execute(ctx context.Context, in ExportUserDataInput) (*ExportUserDataOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return nil, fmt.Errorf("export user data: userID is required")
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Every read below depends only on userID (or userID+today), never on
	// another read's result, so they run concurrently instead of as 9
	// sequential round trips to Neon — wall time collapses to roughly
	// that of the slowest one. Each goroutine writes to its own variable
	// declared below, never one another goroutine also writes, and
	// errgroup's Wait() happens-after every Go() goroutine returns, so
	// every write below is visible once Wait() returns — no data race,
	// no mutex needed. errgroup.WithContext also cancels the other
	// in-flight reads as soon as one fails, instead of letting them run
	// to completion for a result that's getting discarded anyway.
	var (
		user             *domain.User
		consents         map[domain.ConsentPurpose]domain.Consent
		lifestyleChanges []*domain.LifestyleChange
		dailyCheckins    []*domain.DailyCheckin
		sessionLogs      []*domain.SessionLog
		recoveryActions  []*domain.RecoveryAction
		nutritionPlan    *domain.NutritionPlan
		currentDraft     *WeekDraft
		legacyPlan       *domain.Plan
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		if user, err = uc.Users.GetByID(gctx, userID); err != nil {
			return fmt.Errorf("loading profile: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		if consents, err = uc.Consents.GetAll(gctx, userID); err != nil {
			return fmt.Errorf("loading consents: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		if lifestyleChanges, err = uc.LifestyleChanges.ListByUser(gctx, userID, exportLifestyleChangeLimit); err != nil {
			return fmt.Errorf("loading lifestyle changes: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		if dailyCheckins, err = uc.DailyCheckins.ListByUser(gctx, userID, exportDailyRecordLimit); err != nil {
			return fmt.Errorf("loading daily check-ins: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		if sessionLogs, err = uc.SessionLogs.ListByUser(gctx, userID, exportDailyRecordLimit); err != nil {
			return fmt.Errorf("loading session logs: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		if recoveryActions, err = uc.RecoveryActions.ListByUser(gctx, userID, exportDailyRecordLimit); err != nil {
			return fmt.Errorf("loading recovery actions: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		if nutritionPlan, err = uc.NutritionPlans.GetByUserID(gctx, userID); err != nil {
			return fmt.Errorf("loading nutrition plan: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		if currentDraft, err = uc.WeeklyPlanDrafts.GetByDate(gctx, userID, today); err != nil {
			return fmt.Errorf("loading current week: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		if legacyPlan, err = uc.Plans.GetLatest(gctx, userID); err != nil {
			return fmt.Errorf("loading legacy plan: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("export user data: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound(userID)
	}

	return &ExportUserDataOutput{
		ExportedAt:       now,
		Profile:          user,
		Consents:         consents,
		LifestyleChanges: lifestyleChanges,
		DailyCheckins:    dailyCheckins,
		SessionLogs:      sessionLogs,
		RecoveryActions:  recoveryActions,
		NutritionPlan:    nutritionPlan,
		CurrentWeekDraft: currentDraft,
		LegacyPlan:       legacyPlan,
	}, nil
}
