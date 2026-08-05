// Package repository contains dual-write wrappers that implement each usecase
// repository interface by writing to both Firestore (primary) and Neon (secondary).
//
// Strategy:
//   - Writes: always write Firestore first. If Neon fails, log the error but
//     return nil — Firestore is the source of truth during migration. This means
//     the app never degrades because of Neon issues.
//   - Reads: always read from Firestore. Neon reads are not used until Fase 4.
//
// Once you're confident Neon data is complete and correct, flip ReadFrom to Neon
// and drop the Firestore calls.

package repository

import (
	"context"
	"log/slog"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

// ============================================================
// UserRepository dual-write
// ============================================================

var _ usecase.UserRepository = (*DualUserRepository)(nil)

type DualUserRepository struct {
	primary   usecase.UserRepository // Firestore
	secondary usecase.UserRepository // Neon
}

func NewDualUserRepository(primary, secondary usecase.UserRepository) *DualUserRepository {
	return &DualUserRepository{primary: primary, secondary: secondary}
}

func (r *DualUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return r.primary.GetByID(ctx, id) // reads from Firestore
}

func (r *DualUserRepository) Save(ctx context.Context, user *domain.User) error {
	if err := r.primary.Save(ctx, user); err != nil {
		return err // Firestore failure is fatal
	}
	if err := r.secondary.Save(ctx, user); err != nil {
		slog.Error("dual-write: neon user save failed", "user_id", user.ID, "err", err)
		// non-fatal: Firestore already succeeded
	}
	return nil
}

// ============================================================
// CheckinRepository dual-write
// ============================================================

var _ usecase.CheckinRepository = (*DualCheckinRepository)(nil)

type DualCheckinRepository struct {
	primary   usecase.CheckinRepository
	secondary usecase.CheckinRepository
}

func NewDualCheckinRepository(primary, secondary usecase.CheckinRepository) *DualCheckinRepository {
	return &DualCheckinRepository{primary: primary, secondary: secondary}
}

func (r *DualCheckinRepository) Create(ctx context.Context, c *domain.Checkin) error {
	if err := r.primary.Create(ctx, c); err != nil {
		return err
	}
	if err := r.secondary.Create(ctx, c); err != nil {
		slog.Error("dual-write: neon checkin create failed", "checkin_id", c.ID, "err", err)
	}
	return nil
}

func (r *DualCheckinRepository) GetByID(ctx context.Context, userID, id string) (*domain.Checkin, error) {
	return r.primary.GetByID(ctx, userID, id)
}

func (r *DualCheckinRepository) GetLatestByUser(ctx context.Context, userID string) (*domain.Checkin, error) {
	return r.primary.GetLatestByUser(ctx, userID)
}

// ============================================================
// PlanRepository dual-write
// ============================================================

var _ usecase.PlanRepository = (*DualPlanRepository)(nil)

type DualPlanRepository struct {
	primary   usecase.PlanRepository
	secondary usecase.PlanRepository
}

func NewDualPlanRepository(primary, secondary usecase.PlanRepository) *DualPlanRepository {
	return &DualPlanRepository{primary: primary, secondary: secondary}
}

func (r *DualPlanRepository) Create(ctx context.Context, p *domain.Plan) error {
	if err := r.primary.Create(ctx, p); err != nil {
		return err
	}
	if err := r.secondary.Create(ctx, p); err != nil {
		slog.Error("dual-write: neon plan create failed", "plan_id", p.ID, "err", err)
	}
	return nil
}

func (r *DualPlanRepository) GetByID(ctx context.Context, userID, planID string) (*domain.Plan, error) {
	return r.primary.GetByID(ctx, userID, planID)
}

func (r *DualPlanRepository) GetLatestByWeekStart(ctx context.Context, userID string, weekStart time.Time) (*domain.Plan, error) {
	return r.primary.GetLatestByWeekStart(ctx, userID, weekStart)
}

func (r *DualPlanRepository) GetLatest(ctx context.Context, userID string) (*domain.Plan, error) {
	return r.primary.GetLatest(ctx, userID)
}

func (r *DualPlanRepository) UpdateTrainingCompleted(ctx context.Context, userID, planID string, completed map[string]bool) error {
	if err := r.primary.UpdateTrainingCompleted(ctx, userID, planID, completed); err != nil {
		return err
	}
	if err := r.secondary.UpdateTrainingCompleted(ctx, userID, planID, completed); err != nil {
		slog.Error("dual-write: neon plan update training_completed failed", "plan_id", planID, "err", err)
	}
	return nil
}

func (r *DualPlanRepository) UpdatedTrainingJSON(ctx context.Context, userID, planID string, trainingJSON []byte) error {
	if err := r.primary.UpdatedTrainingJSON(ctx, userID, planID, trainingJSON); err != nil {
		return err
	}
	if err := r.secondary.UpdatedTrainingJSON(ctx, userID, planID, trainingJSON); err != nil {
		slog.Error("dual-write: neon plan update training_json failed", "plan_id", planID, "err", err)
	}
	return nil
}

func (r *DualPlanRepository) UpdateNutritionJSON(ctx context.Context, userID, planID string, nutritionJSON []byte) error {
	if err := r.primary.UpdateNutritionJSON(ctx, userID, planID, nutritionJSON); err != nil {
		return err
	}
	if err := r.secondary.UpdateNutritionJSON(ctx, userID, planID, nutritionJSON); err != nil {
		slog.Error("dual-write: neon plan update nutrition_json failed", "plan_id", planID, "err", err)
	}
	return nil
}

func (r *DualPlanRepository) UpdatePhaseFeedback(ctx context.Context, userID, planID string, feedback map[string]domain.PhaseFeedbackEntry) error {
	if err := r.primary.UpdatePhaseFeedback(ctx, userID, planID, feedback); err != nil {
		return err
	}
	if err := r.secondary.UpdatePhaseFeedback(ctx, userID, planID, feedback); err != nil {
		slog.Error("dual-write: neon plan update phase_feedback failed", "plan_id", planID, "err", err)
	}
	return nil
}

func (r *DualPlanRepository) UpdateMealSelections(ctx context.Context, userID, planID string, selections map[string]map[string]int) error {
	if err := r.primary.UpdateMealSelections(ctx, userID, planID, selections); err != nil {
		return err
	}
	if err := r.secondary.UpdateMealSelections(ctx, userID, planID, selections); err != nil {
		slog.Error("dual-write: neon plan update meal_selections failed", "plan_id", planID, "err", err)
	}
	return nil
}

func (r *DualPlanRepository) IsTrainingDay(ctx context.Context, userID, weekday string) (bool, error) {
	return r.primary.IsTrainingDay(ctx, userID, weekday)
}

// ============================================================
// PlanJobsRepository dual-write
// ============================================================

var _ usecase.PlanJobsRepository = (*DualPlanJobsRepository)(nil)

type DualPlanJobsRepository struct {
	primary   usecase.PlanJobsRepository
	secondary usecase.PlanJobsRepository
}

func NewDualPlanJobsRepository(primary, secondary usecase.PlanJobsRepository) *DualPlanJobsRepository {
	return &DualPlanJobsRepository{primary: primary, secondary: secondary}
}

// planJobLinker is implemented by NeonPlanJobsRepository so its row can be
// tagged with the Firestore-generated job ID at creation time. Every Mark*/
// GetByID call after CreateQueued only ever carries that Firestore ID, so
// without this link Neon has no way to find the row it just created.
type planJobLinker interface {
	CreateQueuedLinked(ctx context.Context, userID, checkinID, firestoreJobID string) error
}

func (r *DualPlanJobsRepository) CreateQueued(ctx context.Context, userID, checkinID string) (string, error) {
	jobID, err := r.primary.CreateQueued(ctx, userID, checkinID)
	if err != nil {
		return "", err
	}
	if linker, ok := r.secondary.(planJobLinker); ok {
		if err := linker.CreateQueuedLinked(ctx, userID, checkinID, jobID); err != nil {
			slog.Error("dual-write: neon plan_job create failed", "firestore_job_id", jobID, "err", err)
		}
	} else if _, err := r.secondary.CreateQueued(ctx, userID, checkinID); err != nil {
		slog.Error("dual-write: neon plan_job create failed", "firestore_job_id", jobID, "err", err)
	}
	return jobID, nil
}

func (r *DualPlanJobsRepository) MarkRunning(ctx context.Context, userID, jobID string) error {
	if err := r.primary.MarkRunning(ctx, userID, jobID); err != nil {
		return err
	}
	if err := r.secondary.MarkRunning(ctx, userID, jobID); err != nil {
		slog.Error("dual-write: neon plan_job mark_running failed", "job_id", jobID, "err", err)
	}
	return nil
}

func (r *DualPlanJobsRepository) MarkDone(ctx context.Context, userID, jobID, planID string) error {
	if err := r.primary.MarkDone(ctx, userID, jobID, planID); err != nil {
		return err
	}
	if err := r.secondary.MarkDone(ctx, userID, jobID, planID); err != nil {
		slog.Error("dual-write: neon plan_job mark_done failed", "job_id", jobID, "err", err)
	}
	return nil
}

func (r *DualPlanJobsRepository) MarkFailed(ctx context.Context, userID, jobID, errorMsg string) error {
	if err := r.primary.MarkFailed(ctx, userID, jobID, errorMsg); err != nil {
		return err
	}
	if err := r.secondary.MarkFailed(ctx, userID, jobID, errorMsg); err != nil {
		slog.Error("dual-write: neon plan_job mark_failed failed", "job_id", jobID, "err", err)
	}
	return nil
}

func (r *DualPlanJobsRepository) GetByID(ctx context.Context, userID, jobID string) (*domain.PlanJob, error) {
	return r.primary.GetByID(ctx, userID, jobID)
}

// ============================================================
// DeviceTokenRepository dual-write
// ============================================================

var _ usecase.DeviceTokenRepository = (*DualDeviceTokenRepository)(nil)

type DualDeviceTokenRepository struct {
	primary   usecase.DeviceTokenRepository
	secondary usecase.DeviceTokenRepository
}

func NewDualDeviceTokenRepository(primary, secondary usecase.DeviceTokenRepository) *DualDeviceTokenRepository {
	return &DualDeviceTokenRepository{primary: primary, secondary: secondary}
}

func (r *DualDeviceTokenRepository) Upsert(ctx context.Context, token *domain.DeviceToken) error {
	if err := r.primary.Upsert(ctx, token); err != nil {
		return err
	}
	if err := r.secondary.Upsert(ctx, token); err != nil {
		slog.Error("dual-write: neon device_token upsert failed", "user_id", token.UserID, "err", err)
	}
	return nil
}

func (r *DualDeviceTokenRepository) GetAllActive(ctx context.Context) ([]*domain.DeviceToken, error) {
	return r.primary.GetAllActive(ctx)
}

func (r *DualDeviceTokenRepository) Deactivate(ctx context.Context, userID string) error {
	if err := r.primary.Deactivate(ctx, userID); err != nil {
		return err
	}
	if err := r.secondary.Deactivate(ctx, userID); err != nil {
		slog.Error("dual-write: neon device_token deactivate failed", "user_id", userID, "err", err)
	}
	return nil
}

func (r *DualDeviceTokenRepository) GetAllActiveByTimezone(ctx context.Context) (map[string][]*domain.DeviceToken, error) {
	return r.primary.GetAllActiveByTimezone(ctx)
}
