package parser

import (
	"encoding/json"
	"fmt"

	"viv/internal/adapters/llm/openai/dto"
)

// Only used to detect which concrete struct to unmarshal into.
type trainingTypeEnvelope struct {
	Meta struct {
		PlanType string `json:"plan_type"`
	} `json:"meta"`
}

// ParseTrainingPlan parses the `training` field (raw JSON) and returns a typed dto.TrainingPlan.
func ParseTrainingPlan(raw json.RawMessage) (dto.TrainingPlan, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("training payload is empty")
	}

	// 1) Detect plan_type
	var env trainingTypeEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("unmarshal training envelope: %w", err)
	}
	if env.Meta.PlanType == "" {
		return nil, fmt.Errorf("training.meta.plan_type is missing")
	}

	// 2) Unmarshal into the correct plan struct
	switch env.Meta.PlanType {
	case "full_guidance":
		var p dto.FullGuidancePlan
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("unmarshal full_guidance training: %w", err)
		}
		return p, nil

	case "guided_support":
		var p dto.GuidedSupportPlan
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("unmarshal guided_support training: %w", err)
		}
		return p, nil

	case "light_guidance":
		var p dto.LightGuidancePlan
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("unmarshal light_guidance training: %w", err)
		}
		return p, nil

	default:
		return nil, fmt.Errorf("unknown training plan_type: %q", env.Meta.PlanType)
	}
}
