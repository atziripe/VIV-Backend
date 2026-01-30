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
}

type PlanGenerator interface {
	GeneratePlan(
		ctx context.Context,
		user *domain.User,
		checkin *domain.Checkin,
	) (domain.PlanGenerationResult, error)
}
