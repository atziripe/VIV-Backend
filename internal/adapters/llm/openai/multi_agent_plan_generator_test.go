package openai

import (
	"context"
	"encoding/json"
	"testing"

	openaiapi "github.com/sashabaranov/go-openai"

	"viv/internal/adapters/llm/openai/agents"
	"viv/internal/core/domain"
)

type fakeModelClient struct {
	responses []string
}

func (f *fakeModelClient) Chat(_ context.Context, _ []openaiapi.ChatCompletionMessage) (openaiapi.ChatCompletionResponse, error) {
	content := "{}"
	if len(f.responses) > 0 {
		content = f.responses[0]
		f.responses = f.responses[1:]
	}
	return openaiapi.ChatCompletionResponse{
		Choices: []openaiapi.ChatCompletionChoice{{Message: openaiapi.ChatCompletionMessage{Content: content}}},
	}, nil
}

func (f *fakeModelClient) Model() string { return "fake-model" }

func TestMultiAgentPlanGeneratorReturnsLegacyWrapperShape(t *testing.T) {
	fitnessResp := `{"training":{"week_start":"2026-01-05","week_end":"2026-01-11","meta":{"plan_type":"guided_support"}},"fitness_summary":{"weekly_intensity":"balanced","session_pattern":"alternating","recovery_load":"moderate","carb_demand_pattern":"mixed","key_constraints":["stress"]}}`
	nutritionResp := `{"nutrition":{"guidance_level":"guided_support","weekly_focus":{},"emphasise":[],"de_emphasise":[],"training_recovery_support":[],"physiological_signals":[],"language_style_check":{}},"nutrition_summary":{"fueling_pattern":"stable","meal_timing_pattern":"regular","recovery_nutrition_bias":"supportive","training_support_notes":["align carbs"],"key_constraints":["digestion"]}}`
	reviewResp := `{"training":{"week_start":"2026-01-05","week_end":"2026-01-11","meta":{"plan_type":"guided_support"}}}`
	recoveryResp := `{"recovery":{"recovery_state":{"label":"Stable"},"what_this_feels_like":{"description":"Effort is steady."},"what_to_respect":{"boundary":"Total load tolerance."},"why":{"context":"At this point of the cycle, stability tends to help."},"keep_in_mind":{"anchor":"Consistency compounds."},"optional_insight":{"title":"","body":""}}}`
	composerResp := `{"weekly_headline":"Stable progression week","cycle_phase_summary":"This phase rewards consistency.","cycle_day_range":"Day 12-18","recommendations":[{"title":"Protect consistency","action":"Hold stable pacing.","why":"Supports better adherence."},{"title":"Fuel predictably","action":"Keep regular nutrition rhythm.","why":"Reduces energy volatility."}]}`

	fake := &fakeModelClient{responses: []string{fitnessResp, nutritionResp, reviewResp, recoveryResp, composerResp}}
	g := &MultiAgentPlanGenerator{
		client:         fake,
		promptVersion:  "v1",
		fitnessAgent:   agents.NewFitnessAgent(fake),
		nutritionAgent: agents.NewNutritionAgent(fake),
	}

	user := &domain.User{ID: "u1", TrainingGuidanceLevel: "Guided support", NutritionGuidanceLevel: "Standard", CycleDay: "12"}
	result, err := g.GeneratePlan(context.Background(), user, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Plan == nil {
		t.Fatal("expected plan")
	}
	if len(result.Plan.TrainingJSON) == 0 || len(result.Plan.NutritionJSON) == 0 || len(result.Plan.RecoveryJSON) == 0 {
		t.Fatal("expected training/nutrition/recovery payloads")
	}
	if result.Plan.WeeklyHeadline == "" {
		t.Fatal("expected weekly_headline")
	}
	if len(result.Plan.Recommendations) < 2 {
		t.Fatalf("expected recommendations, got %d", len(result.Plan.Recommendations))
	}

	var training map[string]any
	if err := json.Unmarshal(result.Plan.TrainingJSON, &training); err != nil {
		t.Fatalf("invalid training json: %v", err)
	}
	if _, ok := training["meta"]; !ok {
		t.Fatal("training missing meta")
	}
}
