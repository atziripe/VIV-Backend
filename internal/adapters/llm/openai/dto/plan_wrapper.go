package dto

import "encoding/json"

type GPTPlanJSON struct {
	WeeklyHeadline    string            `json:"weekly_headline"`
	CyclePhaseSummary string            `json:"cycle_phase_summary"`
	CycleDayRange     string            `json:"cycle_day_range"`
	Training          json.RawMessage   `json:"training"`
	Nutrition         json.RawMessage   `json:"nutrition"`
	Recovery          json.RawMessage   `json:"recovery"`
	Recommendations   []Recommendations `json:"recommendations"`
}

type Recommendations struct {
	Title  string `json:"title"`
	Action string `json:"action"`
	Why    string `json:"why"`
}
