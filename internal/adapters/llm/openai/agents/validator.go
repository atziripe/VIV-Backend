package agents

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ValidateFitnessOutput(out FitnessAgentOutput) error {
	if len(out.Training) == 0 {
		return fmt.Errorf("fitness output missing training")
	}

	planType, err := validateTrainingJSON(out.Training)
	if err != nil {
		return err
	}
	if planType == "" {
		return fmt.Errorf("training.meta.plan_type is required")
	}

	return nil
}

func ValidateNutritionOutput(out NutritionAgentOutput, nutritionLevel string) error {
	if len(out.Nutrition) == 0 {
		return fmt.Errorf("nutrition output missing nutrition")
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Nutrition, &payload); err != nil {
		return fmt.Errorf("nutrition payload invalid JSON: %w", err)
	}

	mustHave := func(keys ...string) error {
		for _, k := range keys {
			if _, ok := payload[k]; !ok {
				return fmt.Errorf("nutrition payload missing key %q", k)
			}
		}
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(nutritionLevel)) {
	case "detailed":
		if err := mustHave(
			"week_start",
			"week_end",
			"guidance_level",
			"meta",
			"baseline_principles",
			"physiology_focus",
			"reference_ranges",
			"daily_orientation",
			"support_layer",
			"guardrails",
			"closing_note",
		); err != nil {
			return err
		}
	case "standard":
		if err := mustHave(
			"guidance_level",
			"weekly_focus",
			"emphasise",
			"de_emphasise",
			"training_recovery_support",
			"physiological_signals",
			"language_style_check",
		); err != nil {
			return err
		}
	default:
		if err := mustHave(
			"guidance_level",
			"weekly_lens",
			"physiological_priorities",
			"nutrition_guardrails",
			"signal_to_monitor",
			"language_style_check",
		); err != nil {
			return err
		}
	}

	return nil
}

func validateTrainingJSON(raw json.RawMessage) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("training payload invalid JSON: %w", err)
	}

	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("training payload missing meta")
	}

	planType, _ := meta["plan_type"].(string)
	planType = strings.TrimSpace(planType)
	if planType == "" {
		return "", fmt.Errorf("training.meta.plan_type is required")
	}
	switch planType {
	case "full_guidance", "guided_support", "light_guidance":
	default:
		return "", fmt.Errorf("unknown training plan_type: %q", planType)
	}

	if _, ok := payload["week_start"]; !ok {
		return "", fmt.Errorf("training payload missing week_start")
	}
	if _, ok := payload["week_end"]; !ok {
		return "", fmt.Errorf("training payload missing week_end")
	}

	if planType == "full_guidance" {
		days, ok := payload["days"].([]any)
		if !ok {
			return "", fmt.Errorf("full_guidance training missing days")
		}
		if len(days) != 7 {
			return "", fmt.Errorf("full_guidance training must contain exactly 7 days, got %d", len(days))
		}
	}

	return planType, nil
}
