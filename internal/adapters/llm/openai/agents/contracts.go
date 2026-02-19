package agents

import (
	"context"
	"encoding/json"

	openai "github.com/sashabaranov/go-openai"
)

type ChatClient interface {
	Chat(ctx context.Context, messages []openai.ChatCompletionMessage) (openai.ChatCompletionResponse, error)
}

type FitnessSummary struct {
	WeeklyIntensity  string   `json:"weekly_intensity"`
	SessionPattern   string   `json:"session_pattern"`
	RecoveryLoad     string   `json:"recovery_load"`
	CarbDemandPattern string  `json:"carb_demand_pattern"`
	KeyConstraints   []string `json:"key_constraints"`
}

type NutritionSummary struct {
	FuelingPattern         string   `json:"fueling_pattern"`
	MealTimingPattern      string   `json:"meal_timing_pattern"`
	RecoveryNutritionBias  string   `json:"recovery_nutrition_bias"`
	TrainingSupportNotes   []string `json:"training_support_notes"`
	KeyConstraints         []string `json:"key_constraints"`
}

type FitnessAgentOutput struct {
	Training json.RawMessage `json:"training"`
	Summary  FitnessSummary  `json:"fitness_summary"`
}

type NutritionAgentOutput struct {
	Nutrition json.RawMessage  `json:"nutrition"`
	Summary   NutritionSummary `json:"nutrition_summary"`
}
