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
// Scope, deliberately: this returns the user's *current* state per entity
// (profile, consent status, the active nutrition plan, this week's training
// plan, the legacy plan if still on that pipeline) plus the *full* history
// of lifestyle changes (the one entity with an existing "list everything"
// repository method, LifestyleChangeRepository.ListByUser). It does NOT
// include full historical daily check-ins, session logs, or recovery
// actions — those repositories only expose single-date lookups
// (GetByDate), not a "list every date this user ever had one" method, and
// adding that is real new port surface across both Firestore and Neon, not
// something to bolt on silently inside an export endpoint. Flagged here so
// it's a known v1 scope decision, not an oversight.
// ============================================================================

type ExportUserDataInput struct {
	UserID string
}

type ExportUserDataOutput struct {
	ExportedAt time.Time

	Profile  *domain.User
	Consents map[domain.ConsentPurpose]domain.Consent

	// LifestyleChanges is the user's full history — the one entity below
	// with a real "list everything" repository method already.
	LifestyleChanges []*domain.LifestyleChange

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
	NutritionPlans   NutritionPlanRepository
	WeeklyPlanDrafts WeeklyPlanDraftRepository
	Plans            PlanRepository
}

func NewExportUserDataUseCase(
	users UserRepository,
	consents ConsentRepository,
	lifestyleChanges LifestyleChangeRepository,
	nutritionPlans NutritionPlanRepository,
	weeklyPlanDrafts WeeklyPlanDraftRepository,
	plans PlanRepository,
) *ExportUserDataUseCase {
	return &ExportUserDataUseCase{
		Users: users, Consents: consents, LifestyleChanges: lifestyleChanges,
		NutritionPlans: nutritionPlans, WeeklyPlanDrafts: weeklyPlanDrafts, Plans: plans,
	}
}

// exportLifestyleChangeLimit is generous enough that "full history" is true
// in practice (ListByUser has no unbounded/no-limit mode) without risking
// an unbounded read for a pathological account.
const exportLifestyleChangeLimit = 1000

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
		NutritionPlan:    nutritionPlan,
		CurrentWeekDraft: currentDraft,
		LegacyPlan:       legacyPlan,
	}, nil
}
