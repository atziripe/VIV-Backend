package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"viv/internal/core/domain"
)

type ReportLifestyleChangeInput struct {
	UserID       string
	Type         string // illness | travel | injury | sleep_disruption | other
	SpaceTrain   string // puede entrenar normal/limitado/nada
	PossibleDiet string // puede seguir dieta normal o no
	Energy       string // nivel de energía
	AppliesTo    *time.Time
}

type ReportLifestyleChangeOutput struct {
	Change          *domain.LifestyleChange
	TrainingPaused  bool
	HasActiveInjury bool
	PlanID          *string
}

type ReportLifestyleChangeUseCase struct {
	LifestyleChanges LifestyleChangeRepository
	Users            UserRepository
}

func NewReportLifestyleChangeUseCase(
	lifestyleRepo LifestyleChangeRepository,
	userRepo UserRepository,
) *ReportLifestyleChangeUseCase {
	return &ReportLifestyleChangeUseCase{
		LifestyleChanges: lifestyleRepo,
		Users:            userRepo,
	}
}

func (uc *ReportLifestyleChangeUseCase) Execute(ctx context.Context, in ReportLifestyleChangeInput) (*ReportLifestyleChangeOutput, error) {
	now := time.Now().UTC()

	// 1. Traer user para conocer last_active_plan_id y estado de injury
	user, err := uc.Users.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		// Puedes devolver error propio si quieres
		return nil, ErrUserNotFound(in.UserID)
	}

	var planID *string
	if user.LastActivePlanID != nil {
		planID = user.LastActivePlanID
	}

	change := &domain.LifestyleChange{
		ID:           uuid.NewString(),
		UserID:       in.UserID,
		CreatedAt:    now,
		Type:         in.Type,
		SpaceTrain:   in.SpaceTrain,
		PossibleDiet: in.PossibleDiet,
		Energy:       in.Energy,
		AppliesTo:    in.AppliesTo,
		PlanID:       planID,
	}

	// 2. Si es injury, marcar flags en user
	trainingPaused := user.TrainingPaused
	hasActiveInjury := user.HasActiveInjury
	if in.Type == "injury" {
		user.HasActiveInjury = true
		user.TrainingPaused = true
		t := now
		user.LastInjuryReportedAt = &t

		if err := uc.Users.Save(ctx, user); err != nil {
			return nil, err
		}

		trainingPaused = true
		hasActiveInjury = true
	}

	// 3. Guardar el evento
	if err := uc.LifestyleChanges.Create(ctx, change); err != nil {
		return nil, err
	}

	return &ReportLifestyleChangeOutput{
		Change:          change,
		TrainingPaused:  trainingPaused,
		HasActiveInjury: hasActiveInjury,
		PlanID:          planID,
	}, nil
}
