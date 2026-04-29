package training

import (
	"strings"

	"viv/internal/core/domain"
)

// ============================================================================
// USER PREFERENCE PARSERS
// ============================================================================
// These functions convert raw UI strings stored in domain.User into
// typed values the budget calculator can work with.
//
// The User struct stores onboarding answers as raw strings because
// they were originally sent directly to the LLM. These parsers bridge
// that legacy format to the typed domain model.

// ParsedTrainingPreferences holds the typed version of the user's
// training preferences from onboarding.
type ParsedTrainingPreferences struct {
	SessionsPerWeek int               // target number of sessions
	DurationMinutes int               // preferred session duration
	Modalities      []domain.Modality // selected training types
	Goals           []string          // optimizing_for values
}

// ParseTrainingPreferences converts raw User fields into typed preferences.
func ParseTrainingPreferences(u domain.User) ParsedTrainingPreferences {
	return ParsedTrainingPreferences{
		SessionsPerWeek: parseTrainingOften(u.TrainingOften),
		DurationMinutes: parseTrainingDuration(u.TrainingDuration),
		Modalities:      parseTrainingType(u.TrainingType),
		Goals:           parseTrainingGoals(u.TrainingGoals),
	}
}

// parseTrainingOften converts "1-3 times" etc. to a concrete number.
// Returns the midpoint of the range as the target.
func parseTrainingOften(s string) int {
	switch strings.TrimSpace(s) {
	case "1-3 times":
		return 2
	case "3-5 times/week":
		return 4
	case "Everyday":
		return 6 // not 7 — min_rest_days rule enforces at least 1 rest
	case "Rarely":
		return 1
	default:
		return 3 // safe default
	}
}

// parseTrainingDuration converts "30 - 45 min" etc. to minutes.
// Returns the midpoint of the range.
func parseTrainingDuration(s string) int {
	switch strings.TrimSpace(s) {
	case "- 30 min":
		return 25
	case "30 - 45 min":
		return 40
	case "45 - 60 min":
		return 60
	case "1 - 2 hours":
		return 75
	case "+2 hours":
		return 120
	default:
		return 45 // safe default
	}
}

// parseTrainingType converts "Strength,Pilates,HIIT,Cardio" to []Modality.
func parseTrainingType(s string) []domain.Modality {
	if strings.TrimSpace(s) == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	var modalities []domain.Modality

	for _, p := range parts {
		switch strings.TrimSpace(p) {
		case "Strength":
			modalities = append(modalities, domain.ModalityStrength)
		case "Pilates":
			modalities = append(modalities, domain.ModalityPilates)
		case "HIIT":
			modalities = append(modalities, domain.ModalityHIIT)
		case "Cardio":
			modalities = append(modalities, domain.ModalityCardio)
		}
	}

	return modalities
}

// parseTrainingGoals converts "Energy & Consistency,Strength & Resilience"
// to the normalized goal strings used in the library's optimizing_for field.
func parseTrainingGoals(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	var goals []string

	for _, p := range parts {
		normalized := normalizeGoal(strings.TrimSpace(p))
		if normalized != "" {
			goals = append(goals, normalized)
		}
	}

	return goals
}

// normalizeGoal maps UI labels to the library's optimizing_for values.
func normalizeGoal(uiLabel string) string {
	switch uiLabel {
	case "Strength & Resilience":
		return "strength and resilience"
	case "Maintenance":
		return "maintenance"
	case "Body recomposition":
		return "body recomposition"
	case "Stress regulation":
		return "stress regulation"
	case "Energy & Consistency":
		return "energy and consistency"
	case "Performance & Progression":
		return "performance and progression"
	default:
		return ""
	}
}

// ParseDietRestrictions converts "no meat,no dairy" to a slice.
func ParseDietRestrictions(s string) []string {
	if strings.TrimSpace(s) == "" || strings.ToLower(strings.TrimSpace(s)) == "none" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ParseDigestiveIssues converts "Bloating,IBS" to a slice.
func ParseDigestiveConditions(s string) []string {
	if strings.TrimSpace(s) == "" || strings.ToLower(strings.TrimSpace(s)) == "none" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
