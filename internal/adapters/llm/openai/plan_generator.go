package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	openaiapi "github.com/sashabaranov/go-openai"

	"viv/internal/adapters/llm/openai/dto"
	"viv/internal/adapters/llm/openai/prompts"
	"viv/internal/core/domain"
)

type GPTPlanGenerator struct {
	client        *OpenAIClient
	promptVersion string
}

func NewGPTPlanGenerator(client *OpenAIClient, promptVersion string) *GPTPlanGenerator {
	return &GPTPlanGenerator{
		client:        client,
		promptVersion: promptVersion,
	}
}

func (g *GPTPlanGenerator) GeneratePlan(
	ctx context.Context,
	user *domain.User,
	checkin *domain.Checkin,
) (domain.PlanGenerationResult, error) {
	if user == nil {
		return domain.PlanGenerationResult{}, fmt.Errorf("user is nil")
	}

	checkinID := ""
	if checkin != nil {
		checkinID = checkin.ID
	}

	trainLevel := user.TrainingGuidanceLevel
	nutritionLevelGuidance := user.NutritionGuidanceLevel

	msgs := []openaiapi.ChatCompletionMessage{
		{
			Role:    openaiapi.ChatMessageRoleSystem,
			Content: prompts.SystemPromptVIVV1(),
		},
		{
			Role:    openaiapi.ChatMessageRoleSystem,
			Content: prompts.TrainingPlanPromptVIVV1(trainLevel),
		},
		{
			Role:    openaiapi.ChatMessageRoleSystem,
			Content: prompts.NutritionPlanPromptVIVV1(nutritionLevelGuidance),
		},
		{
			Role:    openaiapi.ChatMessageRoleSystem,
			Content: prompts.RecoveryPlanPromptVIVV1(),
		},
		{
			Role:    openaiapi.ChatMessageRoleUser,
			Content: prompts.BuildUserPromptVIVV1(user, checkin),
		},
	}

	resp, err := g.client.Chat(ctx, msgs)
	if err != nil {
		return domain.PlanGenerationResult{}, err
	}
	if len(resp.Choices) == 0 {
		return domain.PlanGenerationResult{}, errors.New("openai returned 0 choices")
	}

	raw := sanitizeJSON(resp.Choices[0].Message.Content)

	// 1) Unmarshal wrapper DTO
	var out dto.GPTPlanJSON
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return domain.PlanGenerationResult{}, fmt.Errorf("failed to parse gpt JSON: %w", err)
	}

	// 2) Extract training week_start/week_end from raw training payload
	if len(out.Training) == 0 {
		return domain.PlanGenerationResult{}, fmt.Errorf("gpt response missing training payload")
	}

	planStart, planEnd, usedFallback := resolveWeekDates(out.Training, time.Now())
	if usedFallback {
		// opcional: útil en beta para detectar drift
		log.Printf("[plans.generate] week dates missing/invalid, fallback used: %s - %s", planStart.Format("2006-01-02"), planEnd.Format("2006-01-02"))
	}

	// 3) Map recommendations dto -> domain
	recs := make([]domain.Recommendations, 0, len(out.Recommendations))
	for _, r := range out.Recommendations {
		recs = append(recs, domain.Recommendations{
			Title:  r.Title,
			Action: r.Action,
			Why:    r.Why,
		})
	}

	// 4) Build domain plan storing raw payloads
	now := time.Now().UTC()
	plan := &domain.Plan{
		UserID:    user.ID,
		Status:    "active",
		CheckinID: checkinID,
		CreatedAt: now,

		StartDate: planStart,
		EndDate:   planEnd,

		WeeklyHeadline:    out.WeeklyHeadline,
		CyclePhaseSummary: out.CyclePhaseSummary,
		CycleDayRange:     out.CycleDayRange,

		TrainingJSON:  []byte(out.Training),
		NutritionJSON: []byte(out.Nutrition),
		RecoveryJSON:  []byte(out.Recovery),

		Recommendations: recs,

		GeneratedFrom: "openai",
		PlanVersion:   1,
	}

	meta := &domain.PlanGenerationMeta{
		ModelName:     g.client.Model(),
		PromptVersion: g.promptVersion,
		TokensInput:   resp.Usage.PromptTokens,
		TokensOutput:  resp.Usage.CompletionTokens,
	}

	return domain.PlanGenerationResult{
		Plan: plan,
		Meta: meta,
	}, nil
}

func sanitizeJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(s)
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	return s
}
