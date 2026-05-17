package usecase

import (
	"context"
	"strconv"
	"time"
	"viv/internal/core/domain"
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
}

type CompleteOnboardingOutput struct {
	User *domain.User
}

type CompleteOnboardingUseCase struct {
	Users UserRepository
}

func NewCompleteOnboardingUseCase(users UserRepository) *CompleteOnboardingUseCase {
	return &CompleteOnboardingUseCase{Users: users}
}

func (uc *CompleteOnboardingUseCase) Execute(ctx context.Context, in CompleteOnboardingInput) (*CompleteOnboardingOutput, error) {
	now := time.Now().UTC()

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

	user.OnboardingCompleted = true
	user.UpdatedAt = now

	if err := uc.Users.Save(ctx, user); err != nil {
		return nil, err
	}

	return &CompleteOnboardingOutput{User: user}, nil
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
