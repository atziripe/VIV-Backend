package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"viv/internal/core/domain"
)

type AdjustPlanInput struct {
	UserID            string
	LifestyleChangeID string
}

type AdjustPlanOutput struct {
	Plan               *domain.Plan
	CurrentPhase       string
	NextPhase          string
	DaysUntilNextPhase int
}

type AdjustPlanUseCase struct {
	userRepo      UserRepository
	checkinRepo   CheckinRepository
	planRepo      PlanRepository
	lifestyleRepo LifestyleChangeRepository
	planGen       PlanGenerator
}

func NewAdjustPlanUseCase(
	userRepo UserRepository,
	checkinRepo CheckinRepository,
	planRepo PlanRepository,
	lifestyleRepo LifestyleChangeRepository,
	planGen PlanGenerator,
) *AdjustPlanUseCase {
	return &AdjustPlanUseCase{
		userRepo:      userRepo,
		checkinRepo:   checkinRepo,
		planRepo:      planRepo,
		lifestyleRepo: lifestyleRepo,
		planGen:       planGen,
	}
}

func (uc *AdjustPlanUseCase) Execute(ctx context.Context, in AdjustPlanInput) (*AdjustPlanOutput, error) {
	log.Printf("[uc.adjustPlan] start userID=%s changeID=%s\n", in.UserID, in.LifestyleChangeID)

	// 1) User
	user, err := uc.userRepo.GetByID(ctx, in.UserID)
	if err != nil {
		log.Printf("[uc.adjustPlan] failed at <STEP>: %+v\n", err)
		return nil, err
	}
	if user == nil {
		log.Printf("[uc.adjustPlan] failed at <STEP>: %+v\n", err)
		return nil, ErrUserNotFound(in.UserID)
	}

	// 2) Lifestyle change
	ch, err := uc.lifestyleRepo.GetByID(ctx, in.UserID, in.LifestyleChangeID)
	if err != nil {
		log.Printf("[uc.adjustPlan] failed at <STEP>: %+v\n", err)
		return nil, err
	}
	if ch == nil {
		log.Printf("[uc.adjustPlan] failed at <STEP>: %+v\n", err)
		return nil, ErrPlanNotFound(in.LifestyleChangeID) // si ya tienes error dedicado, mejor
	}

	// 3) Latest checkin (base)
	latest, err := uc.checkinRepo.GetLatestByUser(ctx, in.UserID)
	if err != nil {
		log.Printf("[uc.adjustPlan] failed at <STEP>: %+v\n", err)
		return nil, err
	}
	if latest == nil {
		return nil, fmt.Errorf("no checkin found: create a checkin before adjusting plan")
	}

	// 4) Injury rule (seguridad)
	if strings.EqualFold(ch.Type, "injury") {
		now := time.Now().UTC()
		user.HasActiveInjury = true
		user.TrainingPaused = true
		user.LastInjuryReportedAt = &now

		// usa el método que tengas realmente (Save / Update)
		if err := uc.userRepo.Save(ctx, user); err != nil {
			return nil, err
		}
	}

	// 5) Construir user "copy" con contexto del lifestyle change SOLO para el prompt
	userCopy := *user

	// 6) Generar plan
	plan, err := uc.planGen.GeneratePlan(ctx, &userCopy, latest)
	if err != nil {
		log.Printf("[uc.adjustPlan] failed at <STEP>: %+v\n", err)
		return nil, err
	}

	// 7) Garantizar fechas de semana
	start := time.Date(latest.WeekStart.Year(), latest.WeekStart.Month(), latest.WeekStart.Day(), 0, 0, 0, 0, time.UTC)
	plan.Plan.StartDate = start
	plan.Plan.EndDate = start.AddDate(0, 0, 6)

	plan.Plan.UserID = user.ID
	plan.Plan.CheckinID = latest.ID
	plan.Plan.Status = "active"
	plan.Plan.CreatedAt = time.Now().UTC()

	plan.Plan.GeneratedFrom = "lifestyle_change"
	plan.Plan.SourceEventID = ch.ID

	// 8) versionado sencillo (basado en current plan)
	plan.Plan.PlanVersion = 1
	if user.LastActivePlanID != nil && *user.LastActivePlanID != "" {
		cur, _ := uc.planRepo.GetByID(ctx, user.ID, *user.LastActivePlanID)
		if cur != nil && sameDay(cur.StartDate, start) {
			if cur.PlanVersion > 0 {
				plan.Plan.PlanVersion = cur.PlanVersion + 1
			} else {
				plan.Plan.PlanVersion = 2
			}
		}
	}

	// 9) Persistir plan (tu repo ya genera ID con NewDoc)
	if err := uc.planRepo.Create(ctx, plan.Plan); err != nil {
		log.Printf("[uc.adjustPlan] failed at <STEP>: %+v\n", err)
		return nil, err
	}

	// 10) Set current plan
	user.LastActivePlanID = &plan.Plan.ID
	if err := uc.userRepo.Save(ctx, user); err != nil {
		log.Printf("[uc.adjustPlan] failed at <STEP>: %+v\n", err)
		return nil, err
	}

	// 11) Guardar plan_id dentro del lifestyle change (trazabilidad)
	_ = uc.lifestyleRepo.SetPlanID(ctx, user.ID, ch.ID, plan.Plan.ID)

	phase := mapCyclePhase(user.CyclePhase)
	nextPhase := NextPhaseName(phase)
	daysRemaining := DaysUntilNextPhase(user)

	return &AdjustPlanOutput{Plan: plan.Plan,
		CurrentPhase:       string(phase),
		NextPhase:          nextPhase,
		DaysUntilNextPhase: daysRemaining,
	}, nil
}
func formatLifestyleContext(ch *domain.LifestyleChange) string {
	if ch == nil {
		return ""
	}

	// Helpers nil-safe
	str := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return ""
		}
		return strings.TrimSpace(s)
	}

	// Si AppliesTo es *time.Time, lo manejamos; si es time.Time también compila si ajustas abajo.
	applies := ""
	// Caso típico: AppliesTo *time.Time
	if ch.AppliesTo != nil {
		applies = ch.AppliesTo.Format("2006-01-02")
	}

	space := str(ch.SpaceTrain)
	diet := str(ch.PossibleDiet)

	// Energy puede ser int, si no viene, será 0 y está ok.
	return fmt.Sprintf(
		"Recent lifestyle change: type=%s; applies_to=%s; energy=%s; space_train=%s; diet_possible=%s.",
		str(ch.Type), applies, ch.Energy, space, diet,
	)
}

func mergeContext(existing, addition string) string {
	existing = strings.TrimSpace(existing)
	addition = strings.TrimSpace(addition)
	if existing == "" {
		return addition
	}
	if addition == "" {
		return existing
	}
	return existing + " " + addition
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
