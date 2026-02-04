package usecase

import (
	"context"
	"strings"

	"viv/internal/core/domain"
)

// GetPlanGenerationStatusUseCase returns the status of a plan generation job.
type GetPlanGenerationStatusUseCase struct {
	jobsRepo PlanJobsRepository
}

func NewGetPlanGenerationStatusUseCase(jobsRepo PlanJobsRepository) *GetPlanGenerationStatusUseCase {
	return &GetPlanGenerationStatusUseCase{jobsRepo: jobsRepo}
}

// Execute returns (nil, nil) if notI mean `job == nil` when the job does not exist.
func (uc *GetPlanGenerationStatusUseCase) Execute(ctx context.Context, userID, jobID string) (*domain.PlanJob, error) {
	userID = strings.TrimSpace(userID)
	jobID = strings.TrimSpace(jobID)

	return uc.jobsRepo.GetByID(ctx, userID, jobID)
}
