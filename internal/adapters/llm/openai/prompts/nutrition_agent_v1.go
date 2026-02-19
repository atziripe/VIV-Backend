package prompts

import "fmt"

func NutritionAgentPromptV1(nutritionLevel string) string {
	return fmt.Sprintf(`
You are generating ONLY the nutrition agent payload for VIV.
Return ONLY valid JSON with exactly this shape:
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

Rules:
- nutrition must follow the nutrition schema for the user's guidance level.
- Do not output wrapper-level fields like weekly_headline, recommendations, training, or recovery.
- nutrition_summary must be concise and normalized for cross-agent use.
- training_support_notes max 4 short items.
- key_constraints max 5 short items.

%s
`, NutritionPlanPromptVIVV1(nutritionLevel))
}
