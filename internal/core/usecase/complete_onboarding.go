package usecase

import (
	"context"
	"strconv"
	"time"
	"viv/internal/core/domain"
)

type CompleteOnboardingInput struct {
	UserID                string
	Name                  string
	DOB                   string
	WeightKg              float64
	HeightCm              float64
	CycleType             string
	CycleDay              string
	CycleDuration         string
	CyclePhase            string
	PeriodDuration        string
	PMSSymptoms           string
	PeriodDetails         string
	TrainingOften         string
	TrainingDuration      string
	TrainingType          string
	TrainingTime          string
	TrainingGuidanceLevel string
	TrainingGoals         string
	DietRestrictions      string
	DietType              string
	MealsPerDay           string
	SleepWindow           string
	StressLevel           string
	Priority              string
	GuidanceLevel         string
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

	cycleDayInt, _ := strconv.Atoi(in.CycleDay)
	normCycleDuration := normalizeCycleDuration(in.CycleDuration)
	cycleDurationInt, _ := strconv.Atoi(normCycleDuration)
	periodDurationInt, _ := strconv.Atoi(in.PeriodDuration)
	user.DOB = in.DOB
	user.Name = in.Name
	user.WeightKg = in.WeightKg
	user.HeightCm = in.HeightCm
	user.CycleType = in.CycleType
	user.CycleDay = in.CycleDay
	user.CycleDuration = normCycleDuration
	user.CyclePhase = phaseForDay(cycleDayInt, cycleDurationInt, periodDurationInt)
	user.PeriodDuration = in.PeriodDuration
	user.PMSSymptoms = in.PMSSymptoms
	user.PeriodDetails = in.PeriodDetails
	user.TrainingOften = in.TrainingOften
	user.TrainingDuration = in.TrainingDuration
	user.TrainingType = in.TrainingType
	user.TrainingTime = in.TrainingTime
	user.TrainingGuidanceLevel = in.TrainingGuidanceLevel
	user.TrainingGoals = in.TrainingGoals
	user.DietRestrictions = in.DietRestrictions
	user.DietType = in.DietType
	user.MealsPerDay = in.MealsPerDay
	user.SleepWindow = in.SleepWindow
	user.StressLevel = in.StressLevel
	user.Priority = in.Priority
	user.GuidanceLevel = in.GuidanceLevel

	user.OnboardingCompleted = true
	user.UpdatedAt = now

	if err := uc.Users.Save(ctx, user); err != nil {
		return nil, err
	}

	return &CompleteOnboardingOutput{User: user}, nil
}
