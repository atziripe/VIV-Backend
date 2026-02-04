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
	GetByID(ctx context.Context, id string, userID string) (*domain.Checkin, error)
	GetLatestByUser(ctx context.Context, userID string) (*domain.Checkin, error)
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
}

type PlanJobsRepository interface {
	CreateQueued(ctx context.Context, userID, checkinID string) (jobID string, err error)
	MarkRunning(ctx context.Context, userID, jobID string) error
	MarkDone(ctx context.Context, userID, jobID, planID string) error
	MarkFailed(ctx context.Context, userID, jobID, errorMsg string) error
	GetByID(ctx context.Context, userID, jobID string) (*domain.PlanJob, error)
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
