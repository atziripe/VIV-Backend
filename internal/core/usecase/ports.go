package usecase

import (
	"context"
	"time"
	"viv/internal/core/domain"
)

type UserRepository interface {
	GetByID(ctx context.Context, id string) (*domain.User, error)
	Save(ctx context.Context, user *domain.User) error
}

type CheckinRepository interface {
	Create(ctx context.Context, c *domain.Checkin) error
	GetByID(ctx context.Context, userID string, id string) (*domain.Checkin, error)
	GetLatestByUser(ctx context.Context, userID string) (*domain.Checkin, error)
}

type CyclePhaseLookup interface {
	CurrentPhase(ctx context.Context, userID string) (domain.CyclePhase, error)
}

type LifestyleChangeRepository interface {
	Create(ctx context.Context, e *domain.LifestyleChange) error
	ListByUser(ctx context.Context, userID string, limit int) ([]*domain.LifestyleChange, error)
	GetByID(ctx context.Context, userID, changeID string) (*domain.LifestyleChange, error)
	SetPlanID(ctx context.Context, userID, changeID, planID string) error
}

type PlanRepository interface {
	Create(ctx context.Context, p *domain.Plan) error
	GetByID(ctx context.Context, userID, planID string) (*domain.Plan, error)
	GetLatestByWeekStart(ctx context.Context, userID string, weekStart time.Time) (*domain.Plan, error)
	GetLatest(ctx context.Context, userID string) (*domain.Plan, error)
	UpdateTrainingCompleted(ctx context.Context, userID, planID string, completed map[string]bool) error
	UpdatedTrainingJSON(ctx context.Context, userID, trainingPlanID string, trianingJson []byte) error
	UpdateNutritionJSON(ctx context.Context, userID, planID string, nutritionJSON []byte) error
	UpdatePhaseFeedback(ctx context.Context, userID, planID string, feedback map[string]domain.PhaseFeedbackEntry) error
	UpdateMealSelections(ctx context.Context, userID, planID string, selections map[string]map[string]int) error
	IsTrainingDay(ctx context.Context, userID, weekday string) (bool, error)
}

type PlanJobsRepository interface {
	CreateQueued(ctx context.Context, userID, checkinID string) (jobID string, err error)
	MarkRunning(ctx context.Context, userID, jobID string) error
	MarkDone(ctx context.Context, userID, jobID, planID string) error
	MarkFailed(ctx context.Context, userID, jobID, errorMsg string) error
	GetByID(ctx context.Context, userID, jobID string) (*domain.PlanJob, error)
}

// CopyEnricher polishes meal-option copy in the background, after a
// nutrition plan is already saved — see mealgen.AsyncCopyEnricher.
// Mirrors runner.CopyEnricher; kept as a separate local port (rather than
// importing the adapter package) so usecase never depends on adapters.
// Optional on any usecase that embeds it: leave nil to skip enrichment.
type CopyEnricher interface {
	EnrichAsync(userID, planID string, plan domain.NutritionWeekPlan)
}

type PlanGenerator interface {
	GeneratePlan(
		ctx context.Context,
		user *domain.User,
		checkin *domain.Checkin,
	) (domain.PlanGenerationResult, error)
}

type PlanGenerationRunner interface {
	// Run starts the background generation process for a previously created job.
	// It must return immediately (non-blocking).
	Run(userID, jobID, checkinID string)
}

type TrainingStructureGenerator interface {
	GenerateWeekStructure(
		ctx context.Context,
		input StructureGenerationInput,
	) (domain.WeekArrangement, TokenUsage, error)
}

type StructureGenerationInput struct {
	Phase         domain.CyclePhase
	Budget        domain.WeekBudget
	Constraints   []string //human readable spacing rules for the prompt
	RetryFeedback string   // violation messages from a failed attempt ("" on first try)
	ModalitySkip  string   // propagated from checkin.ModalitySkip
}

// MealContentGenerator generates meal options for a week.
// The implementation calls an LLM, but the usecase doesn't know that.
type MealContentGenerator interface {
	GenerateWeekMeals(
		ctx context.Context,
		input MealGenerationInput,
	) ([]DayMealsOutput, TokenUsage, error)
}

// MealGenerationInput is everything the LLM needs to generate meal content.
type MealGenerationInput struct {
	Phase               domain.CyclePhase
	Targets             domain.MacroTargets
	Days                []MealDayInput
	DietRestrictions    []string
	ProteinPreference   string   // "Plant based", "Mixed", "Mostly animal based"
	DigestiveConditions []string // "Bloating", "IBS", etc.
	AppetiteStatus      string   // from weekly checkin
	EatingStyle         string
}

// MealDayInput describes one day for meal generation.
type MealDayInput struct {
	Weekday       string
	IsTrainingDay bool
	SessionTime   string // "morning", "afternoon", "evening", "" for rest
	Macros        domain.DayMacros
	MealSlots     []string // slot types: "breakfast", "lunch", "dinner", "pre_training", "post_training"
}

// DayMealsOutput is the LLM's response for one day.
type DayMealsOutput struct {
	Weekday string            `json:"weekday"`
	Meals   []domain.MealSlot `json:"meals"`
}

type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	Model            string
}

type DeviceTokenRepository interface {
	Upsert(ctx context.Context, token *domain.DeviceToken) error
	GetAllActive(ctx context.Context) ([]*domain.DeviceToken, error)
	Deactivate(ctx context.Context, userID string) error // agregar
	GetAllActiveByTimezone(ctx context.Context) (map[string][]*domain.DeviceToken, error)
}

type NotificationRepository interface {
	SendPush(ctx context.Context, token, title, body string) error
}
