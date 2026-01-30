package openai

import (
	"context"
	"time"

	"viv/internal/core/domain"
)

type DummyPlanGenerator struct{}

func NewDummyPlanGenerator() *DummyPlanGenerator {
	return &DummyPlanGenerator{}
}

func (g *DummyPlanGenerator) GeneratePlan(
	ctx context.Context,
	user *domain.User,
	checkin *domain.Checkin,
) (domain.PlanGenerationResult, error) {
	_ = ctx // not used in dummy

	if user == nil {
		return domain.PlanGenerationResult{}, nil
	}

	now := time.Now().UTC()

	checkinID := ""
	if checkin != nil {
		checkinID = checkin.ID
	}

	// Put a valid week range (Mon-Sun) so your queries/handlers work.
	// You can change this logic if your app defines "week" differently.
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for start.Weekday() != time.Monday {
		start = start.AddDate(0, 0, -1)
	}
	end := start.AddDate(0, 0, 6)

	plan := &domain.Plan{
		UserID:    user.ID,
		Status:    "active",
		CheckinID: checkinID,
		CreatedAt: now,
		StartDate: start,
		EndDate:   end,

		WeeklyHeadline:    "Dummy weekly plan",
		CyclePhaseSummary: "Dummy cycle phase summary",
		CycleDayRange:     "N/A",

		TrainingJSON:  []byte(`{"week_start":"` + start.Format("2006-01-02") + `","week_end":"` + end.Format("2006-01-02") + `","meta":{"plan_type":"dummy"}}`),
		NutritionJSON: []byte(`{"meta":{"plan_type":"dummy"}}`),
		RecoveryJSON:  []byte(`{"meta":{"plan_type":"dummy"}}`),

		Recommendations: []domain.Recommendations{
			{
				Title:  "Keep it simple",
				Action: "Do your usual training at an easy-to-moderate effort.",
				Why:    "This is a placeholder plan while the LLM integration is disabled.",
			},
		},

		GeneratedFrom: "dummy",
		PlanVersion:   1,
	}

	meta := &domain.PlanGenerationMeta{
		ModelName:     "dummy",
		PromptVersion: "v1",
		TokensInput:   0,
		TokensOutput:  0,
	}

	return domain.PlanGenerationResult{
		Plan: plan,
		Meta: meta,
	}, nil
}
