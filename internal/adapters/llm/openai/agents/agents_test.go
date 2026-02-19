package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"viv/internal/core/domain"
)

type fakeChatClient struct {
	responses []string
	calls     [][]openai.ChatCompletionMessage
}

func (f *fakeChatClient) Chat(_ context.Context, messages []openai.ChatCompletionMessage) (openai.ChatCompletionResponse, error) {
	f.calls = append(f.calls, messages)
	if len(f.responses) == 0 {
		return openai.ChatCompletionResponse{Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: "{}"}}}}, nil
	}
	content := f.responses[0]
	f.responses = f.responses[1:]
	return openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: content}}},
	}, nil
}

func mustRawJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(raw)
}

func TestFitnessAgentGenerateParsesEachGuidanceLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		planType string
		training map[string]any
	}{
		{
			name:     "full",
			level:    "Full plan",
			planType: "full_guidance",
			training: map[string]any{"week_start": "2026-01-05", "week_end": "2026-01-11", "meta": map[string]any{"plan_type": "full_guidance"}, "days": []any{1, 2, 3, 4, 5, 6, 7}},
		},
		{
			name:     "moderate",
			level:    "Guided support",
			planType: "guided_support",
			training: map[string]any{"week_start": "2026-01-05", "week_end": "2026-01-11", "meta": map[string]any{"plan_type": "guided_support"}},
		},
		{
			name:     "light",
			level:    "Light guidance",
			planType: "light_guidance",
			training: map[string]any{"week_start": "2026-01-05", "week_end": "2026-01-11", "meta": map[string]any{"plan_type": "light_guidance"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"training": tc.training,
				"fitness_summary": map[string]any{
					"weekly_intensity": "balanced", "session_pattern": "distributed", "recovery_load": "moderate", "carb_demand_pattern": "mixed", "key_constraints": []string{"stress"},
				},
			}
			fake := &fakeChatClient{responses: []string{mustRawJSON(t, payload)}}
			agent := NewFitnessAgent(fake)

			user := &domain.User{TrainingGuidanceLevel: tc.level}
			out, _, err := agent.Generate(context.Background(), user, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			planType, err := validateTrainingJSON(out.Training)
			if err != nil {
				t.Fatalf("unexpected training validation error: %v", err)
			}
			if planType != tc.planType {
				t.Fatalf("plan type mismatch: got %s want %s", planType, tc.planType)
			}
		})
	}
}

func TestNutritionAgentInjectsFitnessSummary(t *testing.T) {
	nutrition := map[string]any{
		"guidance_level": "guided_support", "weekly_focus": map[string]any{}, "emphasise": []any{}, "de_emphasise": []any{},
		"training_recovery_support": []any{}, "physiological_signals": []any{}, "language_style_check": map[string]any{},
	}
	payload := map[string]any{
		"nutrition": nutrition,
		"nutrition_summary": map[string]any{
			"fueling_pattern": "stable", "meal_timing_pattern": "regular", "recovery_nutrition_bias": "anti-inflammatory", "training_support_notes": []string{"match carbs to load"}, "key_constraints": []string{"digestion"},
		},
	}
	fake := &fakeChatClient{responses: []string{mustRawJSON(t, payload)}}
	agent := NewNutritionAgent(fake)
	user := &domain.User{NutritionGuidanceLevel: "Standard"}

	_, _, err := agent.Generate(context.Background(), user, nil, FitnessSummary{WeeklyIntensity: "balanced"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.calls) == 0 {
		t.Fatal("expected at least one call")
	}

	lastMsg := fake.calls[0][len(fake.calls[0])-1].Content
	if !strings.Contains(lastMsg, "fitness_summary") || !strings.Contains(lastMsg, "weekly_intensity") {
		t.Fatalf("expected fitness summary in nutrition prompt context, got: %s", lastMsg)
	}
}

func TestFitnessReviewInjectsNutritionSummary(t *testing.T) {
	reviewPayload := `{"training":{"week_start":"2026-01-05","week_end":"2026-01-11","meta":{"plan_type":"guided_support"}}}`
	fake := &fakeChatClient{responses: []string{reviewPayload}}
	agent := NewFitnessAgent(fake)
	user := &domain.User{}

	_, err := agent.Review(context.Background(), user, nil, NutritionSummary{FuelingPattern: "stable"}, json.RawMessage(`{"week_start":"2026-01-05","week_end":"2026-01-11","meta":{"plan_type":"guided_support"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.calls) == 0 {
		t.Fatal("expected at least one call")
	}

	lastMsg := fake.calls[0][len(fake.calls[0])-1].Content
	if !strings.Contains(lastMsg, "nutrition_summary") || !strings.Contains(lastMsg, "fueling_pattern") {
		t.Fatalf("expected nutrition summary in fitness review context, got: %s", lastMsg)
	}
}
