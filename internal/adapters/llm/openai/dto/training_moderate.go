package dto

type GuidedSupportPlan struct {
	WeekStart         string              `json:"week_start"`
	WeekEnd           string              `json:"week_end"`
	Meta              GuidedSupportMeta   `json:"meta"`
	WeeklyFocus       WeeklyFocus         `json:"weekly_focus"`
	IntensityGuidance IntensityGuidanceGS `json:"intensity_guidance"`
	ModalityGuidance  []ModalityGuidance  `json:"modality_guidance"`
	SelfAdjustment    SelfAdjustment      `json:"self_adjustment"`
	Safety            GuidedSupportSafety `json:"safety"`
}

type GuidedSupportMeta struct {
	PlanType                      string   `json:"plan_type"`      // "guided_support"
	GuidanceLevel                 string   `json:"guidance_level"` // "moderate"
	SelectedModalities            []string `json:"selected_modalities"`
	TrainingOftenPerWeek          int      `json:"training_often_per_week"`
	TrainingDurationPreferenceMin int      `json:"training_duration_preference_min"`
	NotesForUI                    string   `json:"notes_for_ui"`
}

type WeeklyFocus struct {
	Headline      string `json:"headline"`
	PrimaryGoal   string `json:"primary_goal"`
	SecondaryGoal string `json:"secondary_goal"`
	WhyThisWeek   string `json:"why_this_week"`
}

/* ---------------- Intensity guidance ---------------- */

type IntensityGuidanceGS struct {
	OverallLabel        string                  `json:"overall_label"` // "easy|moderate|hard" (en tu ejemplo: "moderate")
	Ceiling             string                  `json:"ceiling"`
	Distribution        []IntensityDistribution `json:"distribution"`
	ClassSelectionRules []ClassSelectionRule    `json:"class_selection_rules"`
	LanguageStyleCheck  LanguageStyleCheck      `json:"language_style_check"`
}

type IntensityDistribution struct {
	Label        string `json:"label"`         // "easy|moderate|hard"
	SharePercent int    `json:"share_percent"` // 0..100
	Description  string `json:"description"`
}

type ClassSelectionRule struct {
	Rule    string `json:"rule"`
	Example string `json:"example"`
}

type LanguageStyleCheck struct {
	DirectionalNotCommands bool `json:"directional_not_commands"`
}

/* ---------------- Modality guidance ---------------- */

type ModalityGuidance struct {
	Modality            string   `json:"modality"` // "strength|hiit|pilates|cardio"
	Theme               string   `json:"theme"`
	HowToApplyInClasses []string `json:"how_to_apply_in_classes"`
	IntensityNotes      string   `json:"intensity_notes"`
	WhatToAvoid         []string `json:"what_to_avoid"`
}

/* ---------------- Self-adjustment ---------------- */

type SelfAdjustment struct {
	IfHighStressOrLowRecovery StressOrRecoveryAdjustment `json:"if_high_stress_or_low_recovery"`
	IfFeelingGreat            FeelingGreatAdjustment     `json:"if_feeling_great"`
}

type StressOrRecoveryAdjustment struct {
	DownshiftSteps []string      `json:"downshift_steps"`
	SwapExamples   []SwapExample `json:"swap_examples"`
}

type SwapExample struct {
	From string `json:"from"`
	To   string `json:"to"`
	Why  string `json:"why"`
}

type FeelingGreatAdjustment struct {
	UpshiftSteps []string `json:"upshift_steps"`
	Guardrails   []string `json:"guardrails"`
}

/* ---------------- Safety ---------------- */

type GuidedSupportSafety struct {
	NoSessionsOrDayStructure       bool `json:"no_sessions_or_day_structure"`
	NoExercisePrescription         bool `json:"no_exercise_prescription"`
	ApplicableToClassBasedTraining bool `json:"applicable_to_class_based_training"`
}
