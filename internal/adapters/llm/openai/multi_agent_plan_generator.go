package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	openaiapi "github.com/sashabaranov/go-openai"

	"viv/internal/adapters/llm/openai/agents"
	"viv/internal/adapters/llm/openai/dto"
	"viv/internal/adapters/llm/openai/prompts"
	"viv/internal/core/domain"
)

type MultiAgentPlanGenerator struct {
	client         modelChatClient
	promptVersion  string
	fitnessAgent   *agents.FitnessAgent
	nutritionAgent *agents.NutritionAgent
}

type modelChatClient interface {
	Chat(ctx context.Context, messages []openaiapi.ChatCompletionMessage) (openaiapi.ChatCompletionResponse, error)
	Model() string
}

func NewMultiAgentPlanGenerator(client *OpenAIClient, promptVersion string) *MultiAgentPlanGenerator {
	return &MultiAgentPlanGenerator{
		client:         client,
		promptVersion:  promptVersion,
		fitnessAgent:   agents.NewFitnessAgent(client),
		nutritionAgent: agents.NewNutritionAgent(client),
	}
}

func (g *MultiAgentPlanGenerator) GeneratePlan(ctx context.Context, user *domain.User, checkin *domain.Checkin) (domain.PlanGenerationResult, error) {
	if user == nil {
		return domain.PlanGenerationResult{}, fmt.Errorf("user is nil")
	}

	checkinID := ""
	if checkin != nil {
		checkinID = checkin.ID
	}

	fitnessOut, fitnessResp, err := g.fitnessAgent.Generate(ctx, user, checkin)
	if err != nil {
		return domain.PlanGenerationResult{}, err
	}

	nutritionOut, nutritionResp, err := g.nutritionAgent.Generate(ctx, user, checkin, fitnessOut.Summary)
	if err != nil {
		return domain.PlanGenerationResult{}, err
	}

	trainingJSON := fitnessOut.Training
	reviewedTraining, err := g.fitnessAgent.Review(ctx, user, checkin, nutritionOut.Summary, fitnessOut.Training)
	if err != nil {
		log.Printf("[plans.generate] fitness review failed, using original training payload: %v", err)
	} else {
		trainingJSON = reviewedTraining
	}

	recoveryJSON, recoveryResp, err := g.generateRecovery(ctx, user, checkin, trainingJSON, nutritionOut.Nutrition)
	if err != nil {
		return domain.PlanGenerationResult{}, err
	}

	composerOut, composerResp, err := g.composeWrapper(ctx, user, checkin, trainingJSON, nutritionOut.Nutrition, recoveryJSON, fitnessOut.Summary, nutritionOut.Summary)
	if err != nil {
		log.Printf("[plans.generate] composer failed, using deterministic fallback: %v", err)
		composerOut = fallbackComposition(user)
	}

	out := dto.GPTPlanJSON{
		WeeklyHeadline:    composerOut.WeeklyHeadline,
		CyclePhaseSummary: composerOut.CyclePhaseSummary,
		CycleDayRange:     composerOut.CycleDayRange,
		Training:          trainingJSON,
		Nutrition:         nutritionOut.Nutrition,
		Recovery:          recoveryJSON,
		Recommendations:   composerOut.Recommendations,
	}

	planStart, planEnd, usedFallback := resolveWeekDates(out.Training, time.Now())
	if usedFallback {
		log.Printf("[plans.generate] week dates missing/invalid, fallback used: %s - %s", planStart.Format("2006-01-02"), planEnd.Format("2006-01-02"))
	}

	recs := make([]domain.Recommendations, 0, len(out.Recommendations))
	for _, r := range out.Recommendations {
		recs = append(recs, domain.Recommendations{Title: r.Title, Action: r.Action, Why: r.Why})
	}

	now := time.Now().UTC()
	plan := &domain.Plan{
		UserID:            user.ID,
		Status:            "active",
		CheckinID:         checkinID,
		CreatedAt:         now,
		StartDate:         planStart,
		EndDate:           planEnd,
		WeeklyHeadline:    out.WeeklyHeadline,
		CyclePhaseSummary: out.CyclePhaseSummary,
		CycleDayRange:     out.CycleDayRange,
		TrainingJSON:      []byte(out.Training),
		NutritionJSON:     []byte(out.Nutrition),
		RecoveryJSON:      []byte(out.Recovery),
		Recommendations:   recs,
		GeneratedFrom:     "openai",
		PlanVersion:       1,
	}

	tokensIn := fitnessResp.Usage.PromptTokens + nutritionResp.Usage.PromptTokens + recoveryResp.Usage.PromptTokens + composerResp.Usage.PromptTokens
	tokensOut := fitnessResp.Usage.CompletionTokens + nutritionResp.Usage.CompletionTokens + recoveryResp.Usage.CompletionTokens + composerResp.Usage.CompletionTokens

	meta := &domain.PlanGenerationMeta{
		ModelName:     g.client.Model(),
		PromptVersion: g.promptVersion,
		TokensInput:   tokensIn,
		TokensOutput:  tokensOut,
	}

	return domain.PlanGenerationResult{Plan: plan, Meta: meta}, nil
}

type wrapperComposition struct {
	WeeklyHeadline    string                `json:"weekly_headline"`
	CyclePhaseSummary string                `json:"cycle_phase_summary"`
	CycleDayRange     string                `json:"cycle_day_range"`
	Recommendations   []dto.Recommendations `json:"recommendations"`
}

func (g *MultiAgentPlanGenerator) generateRecovery(ctx context.Context, user *domain.User, checkin *domain.Checkin, training, nutrition json.RawMessage) (json.RawMessage, openaiapi.ChatCompletionResponse, error) {
	msg := []openaiapi.ChatCompletionMessage{
		{Role: openaiapi.ChatMessageRoleSystem, Content: prompts.SystemPromptVIVV1()},
		{Role: openaiapi.ChatMessageRoleSystem, Content: prompts.RecoveryPlanPromptVIVV1()},
		{Role: openaiapi.ChatMessageRoleUser, Content: prompts.BuildUserPromptVIVV1(user, checkin)},
		{Role: openaiapi.ChatMessageRoleUser, Content: mustJSON(map[string]any{"training": training, "nutrition": nutrition})},
	}

	resp, err := g.client.Chat(ctx, msg)
	if err != nil {
		return nil, openaiapi.ChatCompletionResponse{}, err
	}
	if len(resp.Choices) == 0 {
		return nil, openaiapi.ChatCompletionResponse{}, errors.New("recovery generation returned 0 choices")
	}

	var out struct {
		Recovery json.RawMessage `json:"recovery"`
	}
	if err := json.Unmarshal([]byte(sanitizeJSON(resp.Choices[0].Message.Content)), &out); err != nil {
		return nil, openaiapi.ChatCompletionResponse{}, fmt.Errorf("parse recovery JSON: %w", err)
	}
	if len(out.Recovery) == 0 {
		return nil, openaiapi.ChatCompletionResponse{}, fmt.Errorf("recovery payload missing")
	}
	if !json.Valid(out.Recovery) {
		return nil, openaiapi.ChatCompletionResponse{}, fmt.Errorf("recovery payload invalid JSON")
	}

	return out.Recovery, resp, nil
}

func (g *MultiAgentPlanGenerator) composeWrapper(
	ctx context.Context,
	user *domain.User,
	checkin *domain.Checkin,
	training, nutrition, recovery json.RawMessage,
	fitnessSummary agents.FitnessSummary,
	nutriSummary agents.NutritionSummary,
) (wrapperComposition, openaiapi.ChatCompletionResponse, error) {
	contextBlob := mustJSON(map[string]any{
		"fitness_summary":   fitnessSummary,
		"nutrition_summary": nutriSummary,
		"training":          training,
		"nutrition":         nutrition,
		"recovery":          recovery,
	})

	msg := []openaiapi.ChatCompletionMessage{
		{Role: openaiapi.ChatMessageRoleSystem, Content: prompts.SystemPromptVIVV1()},
		{Role: openaiapi.ChatMessageRoleSystem, Content: prompts.ComposerPromptV1()},
		{Role: openaiapi.ChatMessageRoleUser, Content: prompts.BuildUserPromptVIVV1(user, checkin)},
		{Role: openaiapi.ChatMessageRoleUser, Content: contextBlob},
	}

	resp, err := g.client.Chat(ctx, msg)
	if err != nil {
		return wrapperComposition{}, openaiapi.ChatCompletionResponse{}, err
	}
	if len(resp.Choices) == 0 {
		return wrapperComposition{}, openaiapi.ChatCompletionResponse{}, errors.New("composer returned 0 choices")
	}

	var out wrapperComposition
	if err := json.Unmarshal([]byte(sanitizeJSON(resp.Choices[0].Message.Content)), &out); err != nil {
		return wrapperComposition{}, openaiapi.ChatCompletionResponse{}, fmt.Errorf("parse composer JSON: %w", err)
	}
	if strings.TrimSpace(out.WeeklyHeadline) == "" {
		return wrapperComposition{}, openaiapi.ChatCompletionResponse{}, errors.New("composer missing weekly_headline")
	}
	if len(out.Recommendations) < 2 || len(out.Recommendations) > 3 {
		return wrapperComposition{}, openaiapi.ChatCompletionResponse{}, fmt.Errorf("composer recommendations must be 2-3")
	}

	return out, resp, nil
}

func fallbackComposition(user *domain.User) wrapperComposition {
	headline := "Cycle-aware performance with stable recovery"
	if strings.TrimSpace(user.Priority) != "" {
		headline = strings.TrimSpace(user.Priority)
	}

	summary := "This week favors stable effort, predictable fuel, and recovery-aware pacing."
	if strings.TrimSpace(user.CyclePhase) != "" {
		summary = fmt.Sprintf("Current phase (%s) usually responds best to stable workload and predictable recovery inputs.", strings.TrimSpace(user.CyclePhase))
	}

	return wrapperComposition{
		WeeklyHeadline:    headline,
		CyclePhaseSummary: summary,
		CycleDayRange:     buildCycleDayRange(user.CycleDay),
		Recommendations: []dto.Recommendations{
			{Title: "Protect training quality", Action: "Keep effort controlled on higher-load days and maintain a predictable session rhythm.", Why: "Consistency improves adaptation and reduces stress spillover."},
			{Title: "Anchor daily fueling rhythm", Action: "Use regular meal timing with balanced intake to support energy and recovery across the week.", Why: "Stable intake helps preserve performance tolerance."},
		},
	}
}

func buildCycleDayRange(cycleDay string) string {
	cycleDay = strings.TrimSpace(cycleDay)
	if cycleDay == "" {
		return ""
	}
	day, err := strconv.Atoi(cycleDay)
	if err != nil || day <= 0 {
		return ""
	}
	return fmt.Sprintf("Day %d-%d", day, day+6)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
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
