package agents

import (
	"encoding/json"
	"testing"
)

func TestValidateFitnessOutputFullGuidanceSevenDays(t *testing.T) {
	training := map[string]any{
		"week_start": "2026-01-05",
		"week_end":   "2026-01-11",
		"meta": map[string]any{
			"plan_type": "full_guidance",
		},
		"days": []any{1, 2, 3, 4, 5, 6, 7},
	}
	raw, _ := json.Marshal(training)

	err := ValidateFitnessOutput(FitnessAgentOutput{Training: raw})
	if err != nil {
		t.Fatalf("expected valid fitness output, got error: %v", err)
	}
}

func TestValidateFitnessOutputWrongPlanType(t *testing.T) {
	training := map[string]any{
		"week_start": "2026-01-05",
		"week_end":   "2026-01-11",
		"meta": map[string]any{"plan_type": "unknown_type"},
	}
	raw, _ := json.Marshal(training)

	err := ValidateFitnessOutput(FitnessAgentOutput{Training: raw})
	if err == nil {
		t.Fatal("expected error for unknown plan_type")
	}
}

func TestValidateNutritionOutputByLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
		body  map[string]any
	}{
		{
			name:  "detailed",
			level: "Detailed",
			body: map[string]any{
				"week_start": "2026-01-05", "week_end": "2026-01-11", "guidance_level": "detailed", "meta": map[string]any{},
				"baseline_principles": []any{}, "physiology_focus": map[string]any{}, "reference_ranges": map[string]any{}, "daily_orientation": []any{},
				"support_layer": map[string]any{}, "guardrails": []any{}, "closing_note": "ok",
			},
		},
		{
			name:  "standard",
			level: "Standard",
			body: map[string]any{
				"guidance_level": "guided_support", "weekly_focus": map[string]any{}, "emphasise": []any{}, "de_emphasise": []any{},
				"training_recovery_support": []any{}, "physiological_signals": []any{}, "language_style_check": map[string]any{},
			},
		},
		{
			name:  "light",
			level: "Light",
			body: map[string]any{
				"guidance_level": "light", "weekly_lens": map[string]any{}, "physiological_priorities": []any{},
				"nutrition_guardrails": []any{}, "signal_to_monitor": map[string]any{}, "language_style_check": map[string]any{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.body)
			err := ValidateNutritionOutput(NutritionAgentOutput{Nutrition: raw}, tc.level)
			if err != nil {
				t.Fatalf("expected valid nutrition output for %s, got error: %v", tc.level, err)
			}
		})
	}
}
