package usecase

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"viv/internal/core/domain"
)

// UpdateProfileInput uses pointers so a nil field means "not sent by the
// client", distinguishing that from "sent as empty string" — an explicit
// PATCH semantics, unlike CompleteOnboardingInput which always replaces
// every field.
type UpdateProfileInput struct {
	UserID string

	Name     *string
	WeightKg *float64
	HeightCm *float64

	TrainingOften    *string
	TrainingDuration *string
	TrainingType     *string
	TrainingTime     *string
	TrainingGoals    *string

	DietRestrictions     *string
	DietProteinResources *string
	MealsPerDay          *string
	MealsTimingStability *string
	DigestionConditions  *string
	EatingStyle          *string

	SleepWindow        *string
	SleepContinuity    *string
	RecoveryAfterSleep *string
	DailyActivityLevel *string
	LingeringMarker    *string
	StressReactivity   *string
	StressLevel        *string
	Priority           *string

	// Cycle fields editable from profile settings. Reporting an actual period
	// start (last_cycle_start) is deliberately NOT here — that's a distinct
	// "log my period" action with its own recalibration semantics
	// (ApplyCycleStartOverride, same path create_checkin.go uses), not a
	// settings edit.
	CycleType      *string
	CycleDuration  *string
	PeriodDuration *string

	// ApplyPlanChangesNow opts into an immediate full plan regeneration when
	// CycleDuration/PeriodDuration or a training input
	// (TrainingOften/Duration/Type/Time/Goals) changed. Either category
	// only affects the CURRENT WEEK'S ALREADY-GENERATED plan — the cycle
	// day/phase on the user profile itself always updates regardless (see
	// the unconditional phaseForDay recompute below), this flag only gates
	// whether the active plan document gets regenerated to match.
	// Regenerating replaces it outright, discarding this week's
	// TrainingCompleted/MealSelections/PhaseFeedback, so the client should
	// warn the user about that before sending true. Left false (the
	// default), the edit is still saved and picked up next time a plan is
	// generated instead of rewriting this week's.
	ApplyPlanChangesNow bool
}

type UpdateProfileOutput struct {
	User *domain.User

	// PlanRegenTriggered/PlanRegenJobID are set when a full training+
	// nutrition regen job actually started (CycleDuration/PeriodDuration or
	// a training input changed AND ApplyPlanChangesNow was true) — poll
	// GET /training/generate/status?job_id=... for completion.
	PlanRegenTriggered bool
	PlanRegenJobID     string

	// PlanChangesDeferred is true when a cycle-duration/period-duration or
	// training input changed but ApplyPlanChangesNow was false — the edit
	// is saved (cycle day/phase already updated regardless), but the
	// active plan is untouched until the next generation.
	PlanChangesDeferred bool

	// NutritionTargetsUpdated/NewNutritionTargets are set when weight/height
	// changed and the active plan's nutrition was resolved against the new
	// targets synchronously, before this response.
	NutritionTargetsUpdated bool
	NewNutritionTargets     *domain.MacroTargets

	// CurrentPhase/NextPhase/DaysUntilNextPhase mirror what the plan
	// endpoints (GET /plans/current etc.) already compute live from
	// User.CyclePhase/CycleDay/CycleDuration/PeriodDuration — never stored,
	// just recalculated here too so the client doesn't need a second round
	// trip right after a cycle-affecting PATCH to see the updated summary.
	CurrentPhase       string
	NextPhase          string
	DaysUntilNextPhase int
}

// UpdateProfileUseCase persists profile edits and, depending on which
// fields changed, triggers the downstream recompute the rest of the app
// already relies on — reusing the exact same usecases the normal
// generation flow uses, never duplicating their logic:
//
//   - weight_kg / height_cm feed training.CalculateMacros (via
//     GenerateNutritionPlanUsecase), so the active plan's nutrition
//     targets and meal content are regenerated synchronously, before
//     responding — safe inline because meal content is now a
//     deterministic ingredient/template solve (mealgen), not a live LLM
//     call.
//   - cycle_duration / period_duration always update CycleDay/CyclePhase on
//     the user profile immediately (see the unconditional phaseForDay
//     recompute below) — that tracking data is never gated. Whether to
//     also regenerate the CURRENT WEEK'S plan to match the new phase is a
//     separate decision: phase drives training structure generation and
//     content selection, but regenerating discards this week's
//     TrainingCompleted/MealSelections/PhaseFeedback, so that only
//     happens when the caller opts in via ApplyPlanChangesNow. Left
//     false, the plan is picked up next time it's generated instead —
//     the client should tell the user their cycle settings were saved and
//     ask whether to update this week's plan now or wait.
//     (cycle_type is intentionally excluded from this — it isn't read
//     anywhere in training or nutrition generation today, so regenerating
//     because of it would be pure cost for zero effect.)
//   - training_often / training_duration / training_type / training_time /
//     training_goals are training-generation inputs and go through the
//     exact same ApplyPlanChangesNow-gated regen as cycle settings above,
//     for the same reason (they only affect the plan, not any other
//     profile state, so there's nothing to update "immediately").
//   - everything else (diet/sleep/stress preferences) just persists — it's
//     picked up next time a plan is generated.
//
// Plans/Checkins/Cycle/NutritionGen/PlanStarter are optional (nil-safe):
// when unset, only the plain field persistence happens.
type UpdateProfileUseCase struct {
	Users        UserRepository
	Plans        PlanRepository
	Checkins     CheckinRepository
	Cycle        CyclePhaseLookup
	NutritionGen *GenerateNutritionPlanUsecase
	PlanStarter  *StartPlanGenerationUseCase
	CopyEnricher CopyEnricher
}

func NewUpdateProfileUseCase(
	users UserRepository,
	plans PlanRepository,
	checkins CheckinRepository,
	cycle CyclePhaseLookup,
	nutritionGen *GenerateNutritionPlanUsecase,
	planStarter *StartPlanGenerationUseCase,
	copyEnricher CopyEnricher,
) *UpdateProfileUseCase {
	return &UpdateProfileUseCase{
		Users:        users,
		Plans:        plans,
		Checkins:     checkins,
		Cycle:        cycle,
		NutritionGen: nutritionGen,
		PlanStarter:  planStarter,
		CopyEnricher: copyEnricher,
	}
}

func (uc *UpdateProfileUseCase) Execute(ctx context.Context, in UpdateProfileInput) (*UpdateProfileOutput, error) {
	user, err := uc.Users.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound(in.UserID)
	}

	// Snapshot the fields that gate downstream recompute BEFORE applying
	// any edits. The frontend sends the full profile on every PATCH (not
	// just the fields the user actually touched), so field *presence*
	// can't be used to infer "this changed" — only a real before/after
	// value comparison can.
	oldWeightKg := user.WeightKg
	oldHeightCm := user.HeightCm
	oldCycleDuration := user.CycleDuration
	oldPeriodDuration := user.PeriodDuration
	oldTrainingOften := user.TrainingOften
	oldTrainingDuration := user.TrainingDuration
	oldTrainingType := user.TrainingType
	oldTrainingTime := user.TrainingTime
	oldTrainingGoals := user.TrainingGoals

	if in.Name != nil {
		user.Name = *in.Name
	}
	if in.WeightKg != nil {
		user.WeightKg = *in.WeightKg
	}
	if in.HeightCm != nil {
		user.HeightCm = *in.HeightCm
	}

	if in.TrainingOften != nil {
		user.TrainingOften = *in.TrainingOften
	}
	if in.TrainingDuration != nil {
		user.TrainingDuration = *in.TrainingDuration
	}
	if in.TrainingType != nil {
		user.TrainingType = *in.TrainingType
	}
	if in.TrainingTime != nil {
		user.TrainingTime = *in.TrainingTime
	}
	if in.TrainingGoals != nil {
		user.TrainingGoals = *in.TrainingGoals
	}

	if in.DietRestrictions != nil {
		user.DietRestrictions = *in.DietRestrictions
	}
	if in.DietProteinResources != nil {
		user.DietProteinResources = *in.DietProteinResources
	}
	if in.MealsPerDay != nil {
		user.MealsPerDay = *in.MealsPerDay
	}
	if in.MealsTimingStability != nil {
		user.MealsTimingStability = *in.MealsTimingStability
	}
	if in.DigestionConditions != nil {
		user.DigestionConditions = *in.DigestionConditions
	}
	if in.EatingStyle != nil {
		user.EatingStyle = *in.EatingStyle
	}

	if in.SleepWindow != nil {
		user.SleepWindow = *in.SleepWindow
	}
	if in.SleepContinuity != nil {
		user.SleepContinuity = *in.SleepContinuity
	}
	if in.RecoveryAfterSleep != nil {
		user.RecoveryAfterSleep = *in.RecoveryAfterSleep
	}
	if in.DailyActivityLevel != nil {
		user.DailyActivityLevel = *in.DailyActivityLevel
	}
	if in.LingeringMarker != nil {
		user.LingeringMarker = *in.LingeringMarker
	}
	if in.StressReactivity != nil {
		user.StressReactivity = *in.StressReactivity
	}
	if in.StressLevel != nil {
		user.StressLevel = *in.StressLevel
	}
	if in.Priority != nil {
		user.Priority = *in.Priority
	}

	if in.CycleType != nil {
		user.CycleType = getCycleType(*in.CycleType)
	}
	if in.CycleDuration != nil {
		user.CycleDuration = normalizeCycleDuration(*in.CycleDuration)
	}
	if in.PeriodDuration != nil {
		user.PeriodDuration = *in.PeriodDuration
	}

	now := time.Now().UTC()
	if in.CycleDuration != nil || in.PeriodDuration != nil {
		// Keep the phase consistent with the current cycle day instead of
		// leaving it stale until the next daily sync.
		duration := parseIntDefault(user.CycleDuration, 28)
		period := parseIntDefault(user.PeriodDuration, 5)
		user.CyclePhase = phaseForDay(user.CycleDay, duration, period)
	}

	user.UpdatedAt = now

	if err := uc.Users.Save(ctx, user); err != nil {
		return nil, err
	}

	// Field classification: which downstream recompute (if any) this PATCH
	// needs to kick off. Compares actual before/after values (not field
	// presence — see the snapshot above) so re-saving an unchanged profile
	// never fires an unnecessary recompute or, worse, a full LLM-backed
	// plan regen job. Persistence above already happened and is never
	// rolled back by a recompute failure below — those are best-effort.
	bodyStatsChanged := user.WeightKg != oldWeightKg || user.HeightCm != oldHeightCm
	// cycle_type intentionally not compared here — it isn't read anywhere
	// in training or nutrition generation today, so it can't affect the
	// plan and shouldn't gate a regen decision.
	cycleSettingsChanged := user.CycleDuration != oldCycleDuration ||
		user.PeriodDuration != oldPeriodDuration
	trainingInputsChanged := user.TrainingOften != oldTrainingOften ||
		user.TrainingDuration != oldTrainingDuration ||
		user.TrainingType != oldTrainingType ||
		user.TrainingTime != oldTrainingTime ||
		user.TrainingGoals != oldTrainingGoals
	planRegenNeeded := cycleSettingsChanged || trainingInputsChanged

	out := &UpdateProfileOutput{User: user}

	phase := mapCyclePhase(user.CyclePhase)
	out.CurrentPhase = string(phase)
	out.NextPhase = NextPhaseName(phase)
	out.DaysUntilNextPhase = DaysUntilNextPhase(user)

	if bodyStatsChanged {
		out.NutritionTargetsUpdated, out.NewNutritionTargets = uc.recomputeNutritionSync(ctx, user)
	}

	switch {
	case planRegenNeeded && in.ApplyPlanChangesNow:
		out.PlanRegenTriggered, out.PlanRegenJobID = uc.triggerFullPlanRegenAsync(ctx, user)
	case planRegenNeeded:
		out.PlanChangesDeferred = true
		log.Printf("[update-profile] plan regen deferred (apply_plan_changes_now=false) user=%s", user.ID)
	}

	return out, nil
}

// recomputeNutritionSync regenerates the active plan's macro targets and
// meal content against the just-saved weight/height, via the same
// GenerateNutritionPlanUsecase POST /training/generate uses. Best-effort:
// a user with no active plan yet, or a transient failure here, must never
// fail the profile PATCH — the next real plan generation will use the
// fresh stats anyway. Returns (updated, newTargets) for the response.
func (uc *UpdateProfileUseCase) recomputeNutritionSync(ctx context.Context, user *domain.User) (bool, *domain.MacroTargets) {
	if uc.Plans == nil || uc.Cycle == nil || uc.NutritionGen == nil {
		return false, nil
	}
	if user.LastActivePlanID == nil || *user.LastActivePlanID == "" {
		return false, nil
	}

	plan, err := uc.Plans.GetByID(ctx, user.ID, *user.LastActivePlanID)
	if err != nil || plan == nil {
		log.Printf("[update-profile] macro recompute: loading plan failed user=%s err=%v", user.ID, err)
		return false, nil
	}

	var trainingPlan domain.TrainingWeekPlan
	if err := json.Unmarshal(plan.TrainingJSON, &trainingPlan); err != nil {
		log.Printf("[update-profile] macro recompute: parsing training plan failed user=%s err=%v", user.ID, err)
		return false, nil
	}

	phase, err := uc.Cycle.CurrentPhase(ctx, user.ID)
	if err != nil {
		log.Printf("[update-profile] macro recompute: phase lookup failed user=%s err=%v", user.ID, err)
		return false, nil
	}

	// Checkin is optional here, same as training_runner.go's Run: fall back
	// to a zero-value Checkin rather than treating "no checkin yet" as a
	// reason to skip the recompute entirely.
	var checkin domain.Checkin
	if uc.Checkins != nil {
		if c, err := uc.Checkins.GetLatestByUser(ctx, user.ID); err != nil {
			log.Printf("[update-profile] macro recompute: checkin lookup failed user=%s err=%v", user.ID, err)
		} else if c != nil {
			checkin = *c
		}
	}

	out, err := uc.NutritionGen.Execute(ctx, GenerateNutritionPlanInput{
		User:         *user,
		Checkin:      checkin,
		Phase:        phase,
		TrainingPlan: trainingPlan,
	})
	if err != nil {
		log.Printf("[update-profile] macro recompute: nutrition generation failed user=%s err=%v", user.ID, err)
		return false, nil
	}

	nutritionJSON, err := json.Marshal(out.Plan)
	if err != nil {
		log.Printf("[update-profile] macro recompute: marshal failed user=%s err=%v", user.ID, err)
		return false, nil
	}

	if err := uc.Plans.UpdateNutritionJSON(ctx, user.ID, plan.ID, nutritionJSON); err != nil {
		log.Printf("[update-profile] macro recompute: persist failed user=%s err=%v", user.ID, err)
		return false, nil
	}

	// Same as training_runner.go: the plan above is already correct and
	// saved, this only polishes Name/Summary text later and must never
	// delay this PATCH's response.
	if uc.CopyEnricher != nil {
		uc.CopyEnricher.EnrichAsync(user.ID, plan.ID, out.Plan)
	}

	log.Printf("[update-profile] macro recompute: done user=%s plan=%s", user.ID, plan.ID)
	return true, &out.Plan.Targets
}

// triggerFullPlanRegenAsync kicks off a full training+nutrition
// regeneration job — called when cycle_duration/period_duration or a
// training input changed AND the caller opted in via ApplyPlanChangesNow —
// via the exact same StartPlanGenerationUseCase POST /training/generate
// uses — no duplicated generation logic. Dispatched non-blockingly
// (Execute here only queues the job and hands off to the background
// runner) since training structure generation calls an LLM and must never
// delay this PATCH's response. Returns (started, jobID) for the response.
func (uc *UpdateProfileUseCase) triggerFullPlanRegenAsync(ctx context.Context, user *domain.User) (bool, string) {
	if uc.PlanStarter == nil {
		return false, ""
	}
	if user.LastActivePlanID == nil || *user.LastActivePlanID == "" {
		return false, ""
	}

	// Checkin is optional, same as POST /training/generate: an empty
	// checkinID is valid, StartPlanGenerationUseCase/the runner fall back
	// to a zero-value Checkin rather than requiring one to exist.
	checkinID := ""
	if uc.Checkins != nil {
		if c, err := uc.Checkins.GetLatestByUser(ctx, user.ID); err != nil {
			log.Printf("[update-profile] plan regen: checkin lookup failed user=%s err=%v", user.ID, err)
		} else if c != nil {
			checkinID = c.ID
		}
	}

	jobID, err := uc.PlanStarter.Execute(ctx, user.ID, checkinID)
	if err != nil {
		log.Printf("[update-profile] plan regen: failed to start user=%s err=%v", user.ID, err)
		return false, ""
	}

	log.Printf("[update-profile] plan regen: started user=%s job=%s", user.ID, jobID)
	return true, jobID
}
