package usecase

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/checkin"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
)

type CompleteOnboardingInput struct {
	UserID         string
	Name           string
	DOB            string
	WeightKg       float64
	HeightCm       float64
	CycleType      string
	LastCycleStart time.Time
	CycleDuration  string
	CyclePhase     string
	PeriodDuration string

	TrainingOften    string
	TrainingDuration string
	TrainingType     string
	TrainingTime     string
	TrainingGoals    string

	DietRestrictions     string
	DietProteinResources string
	MealsPerDay          string
	MealsTimingStability string
	DigestionConditions  string
	EatingStyle          string

	SleepWindow        string
	RecoveryAfterSleep string
	SleepContinuity    string
	LingeringMarker    string
	DailyActivityLevel string
	StressReactivity   string
	StressLevel        string
	Priority           string

	// Activities and GoalID are the new-pipeline catalog/goal selections
	// (VIV-101 taxonomy, VIV-102 goal profiles). Raw strings in — each
	// activity id and the goal id are validated against the real
	// taxonomy/profiles below; an unrecognized value rejects the whole
	// request with a clear error rather than being silently dropped.
	// GoalID is deliberately a single string, not a slice: a caller trying
	// to submit more than one goal fails at JSON-decode time already.
	Activities []string
	GoalID     string

	// HealthDataConsent must be true — onboarding persists cycle/training/
	// nutrition data, which needs explicit consent under GDPR Art. 9, not
	// just the usual contract-necessity basis. There's no "onboard without
	// it" path: Execute rejects the request outright (ErrConsentRequired)
	// rather than silently proceeding without a consent record.
	HealthDataConsent bool

	// AIProcessingConsent is optional — see domain.ConsentAIProcessing's
	// doc comment for exactly which OpenAI calls it gates (the weekly note
	// and meal-copy polish, not weekly scheduling). Recorded as whatever
	// the caller sent, true or false, so declining is captured just as
	// explicitly as granting.
	AIProcessingConsent bool
}

type CompleteOnboardingOutput struct {
	User *domain.User

	// WeeklyPlanJobID is the id of the queued first weekly-plan generation
	// (see triggerFirstWeeklyPlanAsync) — poll it via the same
	// GET /training/weekly-plan/generate/status?job_id=... endpoint
	// /training/weekly-plan/generate's job uses. Empty when queuing itself
	// failed or JobsRepo/Runner aren't wired — onboarding still succeeded,
	// there's just nothing to poll.
	WeeklyPlanJobID string
}

// WeeklyPlanGenerator triggers the user's first weekly-plan generation
// (VIV-106) right after onboarding completes. Satisfied directly by
// *GenerateWeeklyPlanUsecase (see main.go's wiring) — declared narrowly
// here so onboarding's own tests don't need that pipeline's five
// collaborators. Optional: leave nil to skip triggering generation
// entirely (matches the CopyEnricher port's optional-collaborator
// convention).
type WeeklyPlanGenerator interface {
	Execute(ctx context.Context, input GenerateWeeklyPlanInput) (GenerateWeeklyPlanOutput, error)
}

// DefaultOnboardingReadiness is the explicit, neutral readiness override
// used for a user's very first weekly-plan generation, triggered right
// after onboarding completes — before any daily check-in exists to derive
// real readiness from (see checkin.Derive). It is a documented first-draft
// default, not a validated assessment of the user, matching the same
// "unremarkable day" philosophy as DefaultDailyCheckin.
var DefaultOnboardingReadiness = checkin.ReadinessDimensions{
	RecoveryCapacity: checkin.RecoveryModerate,
	LifeBandwidth:    checkin.BandwidthModerate,
	BuildReadiness:   checkin.BuildMaintain,
}

type CompleteOnboardingUseCase struct {
	Users    UserRepository
	JobsRepo PlanJobsRepository
	Runner   WeeklyPlanGenerationRunner
	Consents ConsentRepository
}

func NewCompleteOnboardingUseCase(users UserRepository, jobsRepo PlanJobsRepository, runner WeeklyPlanGenerationRunner, consents ConsentRepository) *CompleteOnboardingUseCase {
	return &CompleteOnboardingUseCase{Users: users, JobsRepo: jobsRepo, Runner: runner, Consents: consents}
}

func (uc *CompleteOnboardingUseCase) Execute(ctx context.Context, in CompleteOnboardingInput) (*CompleteOnboardingOutput, error) {
	now := time.Now().UTC()

	if !in.HealthDataConsent {
		return nil, ErrConsentRequired(string(domain.ConsentHealthDataProcessing))
	}

	catalogIDs, err := validateActivityIDs(in.Activities)
	if err != nil {
		return nil, err
	}
	goalID, err := validateGoalID(in.GoalID)
	if err != nil {
		return nil, err
	}

	user, err := uc.Users.GetByID(ctx, in.UserID)
	if err != nil {
		return &CompleteOnboardingOutput{}, ErrUserNotFound(in.UserID)
		// Usuario no encontrado
	}
	if user == nil {
		// Crear nuevo usuario
		user = &domain.User{
			ID:        in.UserID,
			CreatedAt: now,
		}
	}

	cycleDayInt := getCycleDay(in.LastCycleStart)
	normCycleDuration := normalizeCycleDuration(in.CycleDuration)
	cycleDurationInt, _ := strconv.Atoi(normCycleDuration)
	periodDurationInt, _ := strconv.Atoi(in.PeriodDuration)
	user.DOB = in.DOB
	user.Name = in.Name
	user.WeightKg = in.WeightKg
	user.HeightCm = in.HeightCm
	user.CycleType = getCycleType(in.CycleType)
	user.CycleDay = cycleDayInt
	user.CycleDuration = normCycleDuration
	user.CyclePhase = phaseForDay(cycleDayInt, cycleDurationInt, periodDurationInt)
	user.PeriodDuration = in.PeriodDuration
	user.TrainingOften = in.TrainingOften
	user.TrainingDuration = in.TrainingDuration
	user.TrainingType = in.TrainingType
	user.TrainingTime = in.TrainingTime
	user.TrainingGoals = in.TrainingGoals

	user.DietRestrictions = in.DietRestrictions
	user.DietProteinResources = in.DietProteinResources
	user.MealsPerDay = in.MealsPerDay
	user.MealsTimingStability = in.MealsTimingStability
	user.DigestionConditions = in.DigestionConditions
	user.EatingStyle = in.EatingStyle

	user.SleepWindow = in.SleepWindow
	user.RecoveryAfterSleep = in.RecoveryAfterSleep
	user.SleepContinuity = in.SleepContinuity
	user.DailyActivityLevel = in.DailyActivityLevel
	user.LingeringMarker = in.LingeringMarker
	user.StressReactivity = in.StressReactivity
	user.StressLevel = in.StressLevel
	user.Priority = in.Priority

	user.UserCatalog = catalogIDs
	user.GoalID = goalID

	user.OnboardingCompleted = true
	user.UpdatedAt = now

	// Recorded *before* the profile save and treated as fatal: this
	// codebase must never end up with a saved health profile and no
	// record that consent for it was ever captured — the one scenario
	// Art. 7.1's "demonstrate consent was given" exists to prevent. The
	// reverse (consent recorded, profile save then fails) just means the
	// user redoes onboarding, which is harmless.
	if err := uc.Consents.Grant(ctx, user.ID, domain.ConsentHealthDataProcessing, true, domain.CurrentPrivacyPolicyVersion); err != nil {
		return nil, fmt.Errorf("complete onboarding: recording health data consent: %w", err)
	}
	// Best-effort: declining/losing this record only means ai_processing
	// reads back as "not granted" (the safe default), never a compliance
	// gap the way losing the health-data record would be.
	if err := uc.Consents.Grant(ctx, user.ID, domain.ConsentAIProcessing, in.AIProcessingConsent, domain.CurrentPrivacyPolicyVersion); err != nil {
		log.Printf("[onboarding] failed to record ai_processing consent for user %s: %v", user.ID, err)
	}

	if err := uc.Users.Save(ctx, user); err != nil {
		return nil, err
	}

	jobID := uc.triggerFirstWeeklyPlanAsync(ctx, user)

	return &CompleteOnboardingOutput{User: user, WeeklyPlanJobID: jobID}, nil
}

// validateActivityIDs checks every submitted activity id against the real
// taxonomy (VIV-101). An empty/omitted list is left as nil — catalog
// selection is not being made mandatory here — but any unrecognized id
// rejects the whole request rather than silently dropping it.
func validateActivityIDs(raw []string) ([]activity.ID, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	ids := make([]activity.ID, len(raw))
	for i, s := range raw {
		id := activity.ID(s)
		if _, ok := activity.ByID(id); !ok {
			return nil, ErrInvalidActivity(s)
		}
		ids[i] = id
	}
	return ids, nil
}

// validateGoalID checks the submitted goal id against the real goal
// profiles (VIV-102). An empty/omitted value is left as "" — goal
// selection is not being made mandatory here — but an unrecognized id
// rejects the whole request rather than silently dropping it.
func validateGoalID(raw string) (goal.ID, error) {
	if raw == "" {
		return "", nil
	}
	id := goal.ID(raw)
	if _, ok := goal.ByID(id); !ok {
		return "", ErrInvalidGoal(raw)
	}
	return id, nil
}

// triggerFirstWeeklyPlanAsync queues the user's first weekly-plan
// generation (VIV-106) right after onboarding persists, using the
// catalog/goal just saved and DefaultOnboardingReadiness (no check-in
// exists yet to derive real readiness from) — async, via the same
// job/runner pair POST /training/weekly-plan/generate uses (see
// WeeklyPlanGenerationRunner), so POST /onboarding returns as soon as the
// profile is saved instead of blocking on the LLM scheduling call.
// Returns the job id for the client to poll (GET
// /training/weekly-plan/generate/status?job_id=...), or "" when queuing
// itself failed or JobsRepo/Runner aren't wired. Either way this never
// surfaces an error: onboarding has already succeeded and must never be
// rolled back or fail because plan generation had trouble — the user
// still gets a plan on the next regular run.
func (uc *CompleteOnboardingUseCase) triggerFirstWeeklyPlanAsync(ctx context.Context, user *domain.User) string {
	if uc.JobsRepo == nil || uc.Runner == nil {
		return ""
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	readiness := DefaultOnboardingReadiness

	jobID, err := uc.JobsRepo.CreateQueued(ctx, user.ID, "")
	if err != nil {
		log.Printf("[onboarding] failed to queue first weekly-plan generation for user %s: %v", user.ID, err)
		return ""
	}

	uc.Runner.Run(jobID, GenerateWeeklyPlanInput{
		UserID:         user.ID,
		GenerationDate: today,
		GoalID:         user.GoalID,
		Catalog:        cascade.UserCatalog{Activities: user.UserCatalog},
		Readiness:      &readiness,
	})

	return jobID
}

func getCycleType(reqCycleType string) string {
	switch reqCycleType {
	case "I have a natural menstrual cycle":
		return "natural"
	case "I use hormonal contraception, cycle affected":
		return "hormonal_contraception"
	case "I am in menopause":
		return "menopause"
	default:
		return "not_sure"
	}
}

func getCycleDay(lastCycleStart time.Time) int {
	now := time.Now()

	// Normalizamos ambas fechas a medianoche para evitar errores por horas
	start := time.Date(lastCycleStart.Year(), lastCycleStart.Month(), lastCycleStart.Day(), 0, 0, 0, 0, time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	diff := int(today.Sub(start).Hours() / 24)

	cycleDay := diff + 1

	// Edge case
	if cycleDay < 1 {
		cycleDay = 1
	}

	return cycleDay
}
