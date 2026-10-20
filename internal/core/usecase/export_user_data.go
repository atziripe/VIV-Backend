package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

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

	user, err := uc.Users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("export user data: loading profile: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound(userID)
	}

	consents, err := uc.Consents.GetAll(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("export user data: loading consents: %w", err)
	}

	lifestyleChanges, err := uc.LifestyleChanges.ListByUser(ctx, userID, exportLifestyleChangeLimit)
	if err != nil {
		return nil, fmt.Errorf("export user data: loading lifestyle changes: %w", err)
	}

	dailyCheckins, err := uc.DailyCheckins.ListByUser(ctx, userID, exportDailyRecordLimit)
	if err != nil {
		return nil, fmt.Errorf("export user data: loading daily check-ins: %w", err)
	}

	sessionLogs, err := uc.SessionLogs.ListByUser(ctx, userID, exportDailyRecordLimit)
	if err != nil {
		return nil, fmt.Errorf("export user data: loading session logs: %w", err)
	}

	recoveryActions, err := uc.RecoveryActions.ListByUser(ctx, userID, exportDailyRecordLimit)
	if err != nil {
		return nil, fmt.Errorf("export user data: loading recovery actions: %w", err)
	}

	nutritionPlan, err := uc.NutritionPlans.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("export user data: loading nutrition plan: %w", err)
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	currentDraft, err := uc.WeeklyPlanDrafts.GetByDate(ctx, userID, today)
	if err != nil {
		return nil, fmt.Errorf("export user data: loading current week: %w", err)
	}

	legacyPlan, err := uc.Plans.GetLatest(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("export user data: loading legacy plan: %w", err)
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
