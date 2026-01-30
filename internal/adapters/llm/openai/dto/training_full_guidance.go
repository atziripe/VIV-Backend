package dto

type FullGuidancePlan struct {
	WeekStart string           `json:"week_start"`
	WeekEnd   string           `json:"week_end"`
	Meta      FullGuidanceMeta `json:"meta"`
	Overview  WeeklyOverview   `json:"weekly_overview"`
	Days      []TrainingDay    `json:"days"`
	Safety    TrainingSafety   `json:"safety"`
}

type FullGuidanceMeta struct {
	PlanType                      string   `json:"plan_type"`      // "full_guidance"
	GuidanceLevel                 string   `json:"guidance_level"` // "full"
	TrainingOftenPerWeek          int      `json:"training_often_per_week"`
	TrainingDurationPreferenceMin int      `json:"training_duration_preference_min"`
	SelectedModalities            []string `json:"selected_modalities"` // ["strength","cardio",...]
	IntensityBias                 string   `json:"intensity_bias"`      // "performance" | "balanced" | ...
	NotesForUI                    string   `json:"notes_for_ui"`
}

type WeeklyOverview struct {
	Headline                   string                      `json:"headline"`
	Strategy                   string                      `json:"strategy"`
	VolumeIntensityAdjustments []VolumeIntensityAdjustment `json:"volume_intensity_adjustments"`
}

type VolumeIntensityAdjustment struct {
	Trigger    string `json:"trigger"` // "none" | "high_stress" | ...
	Adjustment string `json:"adjustment"`
	Reason     string `json:"reason"`
}

type TrainingDay struct {
	Date      string     `json:"date"`    // "YYYY-MM-DD"
	Weekday   string     `json:"weekday"` // "Monday"..."Sunday"
	IsRestDay bool       `json:"is_rest_day"`
	Session   DaySession `json:"session"`
}

type DaySession struct {
	Modality    string            `json:"modality"` // "Strength" | "Cardio" | "Rest" (en tu ejemplo viene capitalizado)
	Title       string            `json:"title"`
	DurationMin int               `json:"duration_min"`
	Intensity   IntensityGuidance `json:"intensity"`

	Strength StrengthBlock `json:"strength"`
	HIIT     HIITBlock     `json:"hiit"`
	Pilates  PilatesBlock  `json:"pilates"`
	Cardio   CardioBlock   `json:"cardio"`

	LighterAlternative LighterAlternative `json:"lighter_alternative"`
}

type IntensityGuidance struct {
	Label   string `json:"label"`   // "Easy" | "Moderate" | "Hard" (en tu ejemplo capitalizado)
	Ceiling string `json:"ceiling"` // "Top work <= RPE 8", etc.
	Notes   string `json:"notes"`
}

/* ---------------- Strength ---------------- */

type StrengthBlock struct {
	Theme     string             `json:"theme"`
	Warmup    []string           `json:"warmup"`
	Exercises []StrengthExercise `json:"exercises"`
	Finisher  string             `json:"finisher"`
	Cooldown  []string           `json:"cooldown"`
}

type StrengthExercise struct {
	Name    string   `json:"name"`
	Sets    int      `json:"sets"`
	Reps    string   `json:"reps"` // mantiene "10-12 each side"
	Load    LoadSpec `json:"load"`
	RestSec int      `json:"rest_sec"`
	Notes   string   `json:"notes"`
}

type LoadSpec struct {
	Type  string `json:"type"`  // "RPE" | "RIR" | "percent_effort"
	Value string `json:"value"` // "RPE 7-8" | "RIR 2" | "70-80%"
}

/* ---------------- HIIT ---------------- */

type HIITBlock struct {
	ModalityStyle string        `json:"modality_style"` // "bike|run|row|bodyweight|other" (pero aquí viene string)
	Warmup        []string      `json:"warmup"`
	Intervals     HIITIntervals `json:"intervals"`
	Cooldown      []string      `json:"cooldown"`
}

type HIITIntervals struct {
	WorkSec          int    `json:"work_sec"`
	RestSec          int    `json:"rest_sec"`
	Rounds           int    `json:"rounds"`
	IntensityCeiling string `json:"intensity_ceiling"`
	Notes            string `json:"notes"`
}

/* ---------------- Pilates ---------------- */

type PilatesBlock struct {
	Theme    string `json:"theme"`
	Emphasis string `json:"emphasis"`
	Notes    string `json:"notes"`
}

/* ---------------- Cardio ---------------- */

type CardioBlock struct {
	Mode            string `json:"mode"`             // "walk" | "bike" | ...
	Style           string `json:"style"`            // "steady_only"
	TargetIntensity string `json:"target_intensity"` // "easy" | "moderate"
	DurationMin     int    `json:"duration_min"`
	Notes           string `json:"notes"`
}

/* ---------------- Lighter alternative ---------------- */

type LighterAlternative struct {
	Title        string `json:"title"`
	DurationMin  int    `json:"duration_min"`
	Instructions string `json:"instructions"`
}

/* ---------------- Safety ---------------- */

type TrainingSafety struct {
	MaxEffortAvoidance        bool     `json:"max_effort_avoidance"`
	ConstraintsRespected      []string `json:"constraints_respected"`
	InjuryOrPausedRuleApplied bool     `json:"injury_or_paused_rule_applied"`
}
