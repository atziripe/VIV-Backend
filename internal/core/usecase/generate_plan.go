package usecase

import (
	"context"
	"strings"
	"time"

	"viv/internal/core/domain"
)

// GeneratePlanUseCase genera un plan semanal a partir de un check-in
type GeneratePlanUseCase struct {
	userRepo    UserRepository
	checkinRepo CheckinRepository
	planRepo    PlanRepository
	planGen     PlanGenerator
}

func NewGeneratePlanUseCase(
	userRepo UserRepository,
	checkinRepo CheckinRepository,
	planRepo PlanRepository,
	planGen PlanGenerator,
) *GeneratePlanUseCase {
	return &GeneratePlanUseCase{
		userRepo:    userRepo,
		checkinRepo: checkinRepo,
		planRepo:    planRepo,
		planGen:     planGen,
	}
}

func (uc *GeneratePlanUseCase) Generate(
	ctx context.Context,
	userID string,
	checkinID string,
) (domain.PlanGenerationResult, error) {

	// 1️⃣ Obtener usuario
	user, err := uc.userRepo.GetByID(ctx, userID)
	if err != nil {
		return domain.PlanGenerationResult{}, err
	}
	if user == nil {
		return domain.PlanGenerationResult{}, ErrUserNotFound(userID)
	}

	now := time.Now().UTC()

	// 2️⃣ Obtener check-in (opcional)
	var (
		checkin       *domain.Checkin
		generatedFrom string
		sourceEventID string
		status        string
		start         time.Time
		end           time.Time
	)

	checkinID = strings.TrimSpace(checkinID)

	if checkinID != "" {
		// Caso normal: con check-in
		c, err := uc.checkinRepo.GetByID(ctx, checkinID, userID)
		if err != nil {
			return domain.PlanGenerationResult{}, err
		}
		if c == nil {
			// Esto es "checkin not found" (ahora tu código usaba ErrPlanNotFound)
			// Lo dejo como lo tenías para no tocar el resto del sistema.
			return domain.PlanGenerationResult{}, ErrPlanNotFound(checkinID)
		}

		checkin = c
		generatedFrom = "checkin"
		sourceEventID = checkin.ID
		status = "active"

		start = time.Date(checkin.WeekStart.Year(), checkin.WeekStart.Month(), checkin.WeekStart.Day(), 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 0, 6)

	} else {
		// Caso starter: sin check-in (primer plan después del onboarding)
		checkin = nil
		generatedFrom = "onboarding"
		sourceEventID = ""          // no hay evento
		status = "starter"          // o "active" si prefieres
		start = startOfISOWeek(now) // lunes de la semana actual (UTC)
		end = start.AddDate(0, 0, 6)
	}

	// 3️⃣ Generar plan (GPT o dummy) - debe soportar checkin=nil
	plan, err := uc.planGen.GeneratePlan(ctx, user, checkin)
	if err != nil {
		return domain.PlanGenerationResult{}, err
	}

	// 4️⃣ Completar campos de sistema
	plan.Plan.UserID = user.ID
	plan.Plan.Status = status
	plan.Plan.CreatedAt = now
	plan.Plan.StartDate = start
	plan.Plan.EndDate = end

	// Solo setear CheckinID si existe
	if checkin != nil {
		plan.Plan.CheckinID = checkin.ID
	} else {
		plan.Plan.CheckinID = "" // importante para Firestore/JSON, evita basura
	}

	// 6️⃣ Persistir plan
	if err := uc.planRepo.Create(ctx, plan.Plan); err != nil {
		return domain.PlanGenerationResult{}, err
	}

	// 7️⃣ Marcar como plan activo en el usuario
	user.LastActivePlanID = &plan.Plan.ID
	if err := uc.userRepo.Save(ctx, user); err != nil {
		return domain.PlanGenerationResult{}, err
	}

	// 8️⃣ Tracking
	plan.Plan.GeneratedFrom = generatedFrom
	plan.Plan.SourceEventID = sourceEventID
	if plan.Plan.PlanVersion == 0 {
		plan.Plan.PlanVersion = 1
	}

	// guardar plan (solo el plan)
	if err := uc.planRepo.Create(ctx, plan.Plan); err != nil {
		return domain.PlanGenerationResult{}, err
	}

	return plan, nil
}

// startOfISOWeek devuelve el lunes de la semana del timestamp dado (UTC)
func startOfISOWeek(t time.Time) time.Time {
	tt := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	wd := int(tt.Weekday())
	// Go: Sunday=0, Monday=1, ... Saturday=6
	if wd == 0 {
		wd = 7
	}
	// restar (wd-1) días para llegar a lunes
	return tt.AddDate(0, 0, -(wd - 1))
}
