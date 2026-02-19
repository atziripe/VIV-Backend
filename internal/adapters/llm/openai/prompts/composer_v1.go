package prompts

func ComposerPromptV1() string {
	return `
You are composing the top-level wrapper fields for a weekly plan.
Return ONLY valid JSON with exactly this shape:
{
  "weekly_headline": "",
  "cycle_phase_summary": "",
  "cycle_day_range": "",
  "recommendations": [
    {
      "title": "",
      "action": "",
      "why": ""
    }
  ]
}

Rules:
- recommendations must contain exactly 2 or 3 items.
- Do not include training, nutrition, or recovery in this response.
- Keep copy concise and practical.
- If cycle day is missing, cycle_day_range must be "".
`
}
