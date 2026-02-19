package agents

import (
	"context"
	"encoding/json"
	"fmt"

	openaiapi "github.com/sashabaranov/go-openai"

	"viv/internal/adapters/llm/openai/prompts"
	"viv/internal/core/domain"
)

type NutritionAgent struct {
	client ChatClient
}

func NewNutritionAgent(client ChatClient) *NutritionAgent {
	return &NutritionAgent{client: client}
}

func (a *NutritionAgent) Generate(ctx context.Context, user *domain.User, checkin *domain.Checkin, fitnessSummary FitnessSummary) (NutritionAgentOutput, openaiapi.ChatCompletionResponse, error) {
	msg := []openaiapi.ChatCompletionMessage{
		{Role: openaiapi.ChatMessageRoleSystem, Content: prompts.SystemPromptVIVV1()},
		{Role: openaiapi.ChatMessageRoleSystem, Content: prompts.NutritionAgentPromptV1(user.NutritionGuidanceLevel)},
		{Role: openaiapi.ChatMessageRoleUser, Content: prompts.BuildUserPromptVIVV1(user, checkin)},
		{Role: openaiapi.ChatMessageRoleUser, Content: mustJSON(map[string]any{"fitness_summary": fitnessSummary})},
	}

	resp, out, err := a.chatAndParse(ctx, msg)
	if err == nil && ValidateNutritionOutput(out, user.NutritionGuidanceLevel) == nil {
		return out, resp, nil
	}

	// Repair attempt (single retry)
	repairMsg := append(msg,
		openaiapi.ChatCompletionMessage{Role: openaiapi.ChatMessageRoleSystem, Content: nutritionRepairPrompt(user.NutritionGuidanceLevel)},
		openaiapi.ChatCompletionMessage{Role: openaiapi.ChatMessageRoleUser, Content: "Repair and return valid JSON only."},
	)

	repairResp, repairOut, repairErr := a.chatAndParse(ctx, repairMsg)
	if repairErr != nil {
		if err != nil {
			return NutritionAgentOutput{}, openaiapi.ChatCompletionResponse{}, err
		}
		return NutritionAgentOutput{}, openaiapi.ChatCompletionResponse{}, repairErr
	}
	if verr := ValidateNutritionOutput(repairOut, user.NutritionGuidanceLevel); verr != nil {
		return NutritionAgentOutput{}, openaiapi.ChatCompletionResponse{}, verr
	}

	return repairOut, repairResp, nil
}

func (a *NutritionAgent) chatAndParse(ctx context.Context, msg []openaiapi.ChatCompletionMessage) (openaiapi.ChatCompletionResponse, NutritionAgentOutput, error) {
	resp, err := a.client.Chat(ctx, msg)
	if err != nil {
		return openaiapi.ChatCompletionResponse{}, NutritionAgentOutput{}, err
	}
	if len(resp.Choices) == 0 {
		return openaiapi.ChatCompletionResponse{}, NutritionAgentOutput{}, fmt.Errorf("nutrition agent returned 0 choices")
	}

	var out NutritionAgentOutput
	if err := json.Unmarshal([]byte(sanitizeJSON(resp.Choices[0].Message.Content)), &out); err != nil {
		return openaiapi.ChatCompletionResponse{}, NutritionAgentOutput{}, fmt.Errorf("parse nutrition agent output: %w", err)
	}

	return resp, out, nil
}

func nutritionRepairPrompt(level string) string {
	return fmt.Sprintf(`Your previous output failed schema validation for nutrition guidance level %q.
Return valid JSON with exactly this shape:
{
  "nutrition": { ... },
  "nutrition_summary": {
    "fueling_pattern": "",
    "meal_timing_pattern": "",
    "recovery_nutrition_bias": "",
    "training_support_notes": [""],
    "key_constraints": [""]
  }
}
Do not add any other keys.`, level)
}
