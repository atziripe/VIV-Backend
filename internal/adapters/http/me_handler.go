package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

type MeHandler struct {
	UC           *usecase.GetCurrentUserUseCase
	UpdateProfUC *usecase.UpdateProfileUseCase
}

func NewMeHandler(uc *usecase.GetCurrentUserUseCase, updateProfUC *usecase.UpdateProfileUseCase) *MeHandler {
	return &MeHandler{UC: uc, UpdateProfUC: updateProfUC}
}

// Lo que devolvemos al frontend (puedes ajustar campos)
type meResponse struct {
	ID                     string `json:"id"`
	Username               string `json:"username"`
	Name                   string `json:"name"`
	OnboardingCompleted    bool   `json:"onboarding_completed"`
	OnboardingCompletedInt int    `json:"onboarding_completed_int"`

	DOB            string  `json:"dob"`
	WeightKg       float64 `json:"weight_kg"`
	HeightCm       float64 `json:"height_cm"`
	CycleType      string  `json:"cycle_type"`
	CycleDay       int     `json:"cycle_day"`
	CyclePhase     string  `json:"cycle_phase"`
	CycleLength    string  `json:"cycle_duration"`
	PeriodDuration string  `json:"period_duration"`

	TrainingOften    string `json:"training_often"`
	TrainingDuration string `json:"training_duration"`
	TrainingType     string `json:"training_type"`
	TrainingTime     string `json:"training_time"`
	TrainingGoals    string `json:"training_goals"`

	DietRestrictions     string `json:"diet_restrictions"`
	DietProteinResources string `json:"diet_protein_resources"`
	MealsPerDay          string `json:"meals_per_day"`
	MealsTimingStability string `json:"meals_timing_stability"`
	DigestionConditions  string `json:"digestion_conditions"`

	SleepWindow        string `json:"sleep_window"`
	RecoveryAfterSleep string `json:"recovery_after_sleep"`
	SleepContinuity    string `json:"sleep_continuity"`
	DailyActivityLevel string `json:"daily_activity_level"`
	LingeringMarker    string `json:"lingering_marker"`
	StressReactivity   string `json:"stress_reactivity"`
	StressLevel        string `json:"stress_level"`
	Priority           string `json:"priority"`

	HasActiveInjury      bool    `json:"has_active_injury"`
	TrainingPaused       bool    `json:"training_paused"`
	LastActivePlanID     *string `json:"last_active_plan_id,omitempty"`
	LastInjuryReportedAt *string `json:"last_injury_reported_at,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (h *MeHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	in := usecase.GetCurrentUserInput{
		UserID: userID,
	}

	out, err := h.UC.Execute(ctx, in)
	if err != nil {
		// MVP error handling, check it later
		log.Printf("http server error: %v", err)
		http.Error(w, "failed to get current user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toMeResponse(out.User))
}

func toMeResponse(u *domain.User) meResponse {
	var lastInjuryStr *string
	if u.LastInjuryReportedAt != nil {
		s := u.LastInjuryReportedAt.UTC().Format("2006-01-02")
		lastInjuryStr = &s
	}

	createdStr := u.CreatedAt.UTC().Format(time.RFC3339)
	updatedStr := u.UpdatedAt.UTC().Format(time.RFC3339)
	onboardingInt := 0
	if u.OnboardingCompleted {
		onboardingInt = 1
	}

	return meResponse{
		ID:                     u.ID,
		Username:               u.Username,
		Name:                   u.Name,
		OnboardingCompleted:    u.OnboardingCompleted,
		OnboardingCompletedInt: onboardingInt,

		DOB:            u.DOB,
		WeightKg:       u.WeightKg,
		HeightCm:       u.HeightCm,
		CycleType:      u.CycleType,
		CycleDay:       u.CycleDay,
		CyclePhase:     u.CyclePhase,
		CycleLength:    u.CycleDuration,
		PeriodDuration: u.PeriodDuration,

		TrainingOften:    u.TrainingOften,
		TrainingDuration: u.TrainingDuration,
		TrainingType:     u.TrainingType,
		TrainingTime:     u.TrainingTime,
		TrainingGoals:    u.TrainingGoals,

		DietRestrictions:     u.DietRestrictions,
		DietProteinResources: u.DietProteinResources,
		MealsPerDay:          u.MealsPerDay,
		MealsTimingStability: u.MealsTimingStability,
		DigestionConditions:  u.DigestionConditions,

		SleepWindow:        u.SleepWindow,
		RecoveryAfterSleep: u.RecoveryAfterSleep,
		SleepContinuity:    u.SleepContinuity,
		LingeringMarker:    u.LingeringMarker,
		DailyActivityLevel: u.DailyActivityLevel,
		StressReactivity:   u.StressReactivity,
		StressLevel:        u.StressLevel,
		Priority:           u.Priority,

		HasActiveInjury:      u.HasActiveInjury,
		TrainingPaused:       u.TrainingPaused,
		LastActivePlanID:     u.LastActivePlanID,
		LastInjuryReportedAt: lastInjuryStr,

		CreatedAt: createdStr,
		UpdatedAt: updatedStr,
	}
}

// updateProfileRequest uses pointers so JSON omission means "leave
// unchanged" — the request body only needs to carry the fields the client
// actually wants to edit.
type updateProfileRequest struct {
	Name     *string  `json:"name"`
	WeightKg *float64 `json:"weight_kg"`
	HeightCm *float64 `json:"height_cm"`

	TrainingOften    *string `json:"training_often"`
	TrainingDuration *string `json:"training_duration"`
	TrainingType     *string `json:"training_type"`
	TrainingTime     *string `json:"training_time"`
	TrainingGoals    *string `json:"training_goals"`

	DietRestrictions     *string `json:"diet_restrictions"`
	DietProteinResources *string `json:"diet_protein_resources"`
	MealsPerDay          *string `json:"meals_per_day"`
	MealsTimingStability *string `json:"meals_timing_stability"`
	DigestionConditions  *string `json:"digestion_conditions"`
	EatingStyle          *string `json:"eating_style"`

	SleepWindow        *string `json:"sleep_window"`
	SleepContinuity    *string `json:"sleep_continuity"`
	RecoveryAfterSleep *string `json:"recovery_after_sleep"`
	DailyActivityLevel *string `json:"daily_activity_level"`
	LingeringMarker    *string `json:"lingering_marker"`
	StressReactivity   *string `json:"stress_reactivity"`
	StressLevel        *string `json:"stress_level"`
	Priority           *string `json:"priority"`

	// last_cycle_start is intentionally not editable here. Reporting an
	// actual period start is a distinct "log my period" action, not a
	// settings edit.
	CycleType      *string `json:"cycle_type"`
	CycleDuration  *string `json:"cycle_duration"`
	PeriodDuration *string `json:"period_duration"`

	// ApplyPlanChangesNow opts into an immediate full plan regeneration when
	// cycle_duration/period_duration or a training input above actually
	// changed value. Defaults to false (omitted = false): the edit is
	// always saved (cycle day/phase tracking updates regardless), but
	// regenerating the active plan replaces it outright, discarding this
	// week's completed days/meal selections — the client should warn the
	// user about that before sending true.
	ApplyPlanChangesNow bool `json:"apply_plan_changes_now"`
}

// immediateEffects reports what the PATCH already did (or started) so the
// client knows which banner/navigation to show — see update_profile.go's
// UpdateProfileOutput for what each field means.
type immediateEffects struct {
	NutritionTargetsUpdated bool                 `json:"nutrition_targets_updated"`
	NewTargets              *domain.MacroTargets `json:"new_targets,omitempty"`

	PlanRegenerating bool   `json:"plan_regenerating"`
	JobID            string `json:"job_id,omitempty"`

	PlanChangesDeferred bool `json:"plan_changes_deferred"`
}

// cycleSummary mirrors the same fields the plan endpoints (GET /plans/current
// etc.) already return — recomputed here too so the client sees the updated
// phase/countdown right after a cycle-affecting PATCH without a second call.
type cycleSummary struct {
	CurrentPhase       string `json:"current_phase"`
	NextPhase          string `json:"next_phase"`
	DaysUntilNextPhase int    `json:"days_until_next_phase"`
}

type updateProfileResponse struct {
	meResponse
	ImmediateEffects immediateEffects `json:"immediate_effects"`
	CycleSummary     cycleSummary     `json:"cycle_summary"`
}

func (h *MeHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[update-profile] decode error: %v", err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	in := usecase.UpdateProfileInput{
		UserID:   userID,
		Name:     req.Name,
		WeightKg: req.WeightKg,
		HeightCm: req.HeightCm,

		TrainingOften:    req.TrainingOften,
		TrainingDuration: req.TrainingDuration,
		TrainingType:     req.TrainingType,
		TrainingTime:     req.TrainingTime,
		TrainingGoals:    req.TrainingGoals,

		DietRestrictions:     req.DietRestrictions,
		DietProteinResources: req.DietProteinResources,
		MealsPerDay:          req.MealsPerDay,
		MealsTimingStability: req.MealsTimingStability,
		DigestionConditions:  req.DigestionConditions,
		EatingStyle:          req.EatingStyle,

		SleepWindow:        req.SleepWindow,
		SleepContinuity:    req.SleepContinuity,
		RecoveryAfterSleep: req.RecoveryAfterSleep,
		DailyActivityLevel: req.DailyActivityLevel,
		LingeringMarker:    req.LingeringMarker,
		StressReactivity:   req.StressReactivity,
		StressLevel:        req.StressLevel,
		Priority:           req.Priority,

		CycleType:      req.CycleType,
		CycleDuration:  req.CycleDuration,
		PeriodDuration: req.PeriodDuration,

		ApplyPlanChangesNow: req.ApplyPlanChangesNow,
	}

	out, err := h.UpdateProfUC.Execute(ctx, in)
	if err != nil {
		var notFound usecase.UserNotFoundError
		if errors.As(err, &notFound) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		log.Printf("[update-profile] execute error: %v", err)
		http.Error(w, "failed to update profile", http.StatusInternalServerError)
		return
	}

	resp := updateProfileResponse{
		meResponse: toMeResponse(out.User),
		ImmediateEffects: immediateEffects{
			NutritionTargetsUpdated: out.NutritionTargetsUpdated,
			NewTargets:              out.NewNutritionTargets,
			PlanRegenerating:        out.PlanRegenTriggered,
			JobID:                   out.PlanRegenJobID,
			PlanChangesDeferred:     out.PlanChangesDeferred,
		},
		CycleSummary: cycleSummary{
			CurrentPhase:       out.CurrentPhase,
			NextPhase:          out.NextPhase,
			DaysUntilNextPhase: out.DaysUntilNextPhase,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
