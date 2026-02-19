package prompts

import "fmt"

func FitnessAgentPromptV1(trainingLevel string) string {
	return fmt.Sprintf(`
You are generating ONLY the fitness agent payload for VIV.
Return ONLY valid JSON with exactly this shape:
{
  "training": { ... },
  "fitness_summary": {
    "weekly_intensity": "",
    "session_pattern": "",
    "recovery_load": "",
    "carb_demand_pattern": "",
    "key_constraints": [""]
  }
}

Rules:
- training must follow the training schema for the user's guidance level.
- Do not output wrapper-level fields like weekly_headline, recommendations, nutrition, or recovery.
- fitness_summary must stay concise and normalized for cross-agent use.
- key_constraints max 5 short items.

%s
`, TrainingPlanPromptVIVV1(trainingLevel))
}

func FitnessReviewPromptV1() string {
	return `
You are reviewing a generated training plan for consistency with nutrition context.
Return ONLY valid JSON with exactly this shape:
{
  "training": { ... }
}

Rules:
- Keep training schema and plan_type unchanged.
- Apply only minimal edits needed for consistency with nutrition summary.
- Never add wrapper-level keys.
- Never modify week_start/week_end unless missing or invalid.
`
}
