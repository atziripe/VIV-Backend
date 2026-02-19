package agents

import (
	"context"
	"encoding/json"
	"fmt"

	openaiapi "github.com/sashabaranov/go-openai"

	"viv/internal/adapters/llm/openai/prompts"
	"viv/internal/core/domain"
)

type FitnessAgent struct {
	client ChatClient
}

func NewFitnessAgent(client ChatClient) *FitnessAgent {
	return &FitnessAgent{client: client}
}

func (a *FitnessAgent) Generate(ctx context.Context, user *domain.User, checkin *domain.Checkin) (FitnessAgentOutput, openaiapi.ChatCompletionResponse, error) {
	msg := []openaiapi.ChatCompletionMessage{
		{Role: openaiapi.ChatMessageRoleSystem, Content: prompts.SystemPromptVIVV1()},
		{Role: openaiapi.ChatMessageRoleSystem, Content: prompts.FitnessAgentPromptV1(user.TrainingGuidanceLevel)},
		{Role: openaiapi.ChatMessageRoleUser, Content: prompts.BuildUserPromptVIVV1(user, checkin)},
	}

	resp, err := a.client.Chat(ctx, msg)
	if err != nil {
		return FitnessAgentOutput{}, openaiapi.ChatCompletionResponse{}, err
	}
	if len(resp.Choices) == 0 {
		return FitnessAgentOutput{}, openaiapi.ChatCompletionResponse{}, fmt.Errorf("fitness agent returned 0 choices")
	}

	var out FitnessAgentOutput
	if err := json.Unmarshal([]byte(sanitizeJSON(resp.Choices[0].Message.Content)), &out); err != nil {
		return FitnessAgentOutput{}, openaiapi.ChatCompletionResponse{}, fmt.Errorf("parse fitness agent output: %w", err)
	}
	if err := ValidateFitnessOutput(out); err != nil {
		return FitnessAgentOutput{}, openaiapi.ChatCompletionResponse{}, err
	}

	return out, resp, nil
}

func (a *FitnessAgent) Review(ctx context.Context, user *domain.User, checkin *domain.Checkin, nutritionSummary NutritionSummary, currentTraining json.RawMessage) (json.RawMessage, error) {
	payload := map[string]any{
		"nutrition_summary": nutritionSummary,
		"training":          json.RawMessage(currentTraining),
	}

	msg := []openaiapi.ChatCompletionMessage{
		{Role: openaiapi.ChatMessageRoleSystem, Content: prompts.SystemPromptVIVV1()},
		{Role: openaiapi.ChatMessageRoleSystem, Content: prompts.FitnessReviewPromptV1()},
		{Role: openaiapi.ChatMessageRoleUser, Content: prompts.BuildUserPromptVIVV1(user, checkin)},
		{Role: openaiapi.ChatMessageRoleUser, Content: mustJSON(payload)},
	}

	resp, err := a.client.Chat(ctx, msg)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("fitness review returned 0 choices")
	}

	var out struct {
		Training json.RawMessage `json:"training"`
	}
	if err := json.Unmarshal([]byte(sanitizeJSON(resp.Choices[0].Message.Content)), &out); err != nil {
		return nil, fmt.Errorf("parse fitness review output: %w", err)
	}
	if len(out.Training) == 0 {
		return nil, fmt.Errorf("fitness review missing training")
	}
	if _, err := validateTrainingJSON(out.Training); err != nil {
		return nil, err
	}

	return out.Training, nil
}
