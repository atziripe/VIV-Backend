package dto

type LightGuidancePlan struct {
	WeekStart   string            `json:"week_start"`
	WeekEnd     string            `json:"week_end"`
	Meta        LightGuidanceMeta `json:"meta"`
	WeeklyLens  WeeklyLens        `json:"weekly_lens"`
	Priorities  []LightItem       `json:"priorities"`
	Guardrails  []GuardrailItem   `json:"guardrails"`
	ClosingNote string            `json:"closing_note"`
}

type LightGuidanceMeta struct {
	PlanType      string `json:"plan_type"`      // "light_guidance"
	GuidanceLevel string `json:"guidance_level"` // "light"
	Domain        string `json:"domain"`         // "training"
	NotesForUI    string `json:"notes_for_ui"`
}

type WeeklyLens struct {
	Sentence string `json:"sentence"`
}

type LightItem struct {
	Label       string `json:"label"`
	Explanation string `json:"explanation"`
}

type GuardrailItem struct {
	Label string `json:"label"`
	Why   string `json:"why"`
}
