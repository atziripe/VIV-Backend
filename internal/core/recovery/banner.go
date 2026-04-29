package recovery

import (
	"fmt"
	"strings"
	"time"

	"viv/internal/core/domain"
)

// ============================================================================
// PHASE EXPERIENCE FLAGS — hardcoded per spec v2
// ============================================================================

var phaseExperienceFlags = map[domain.CyclePhase][]string{
	domain.PhaseMenstrual:   {"blood_loss", "prostaglandin_load", "low_hormones", "iron_depletion"},
	domain.PhaseFollicular:  {"lower_perceived_soreness", "higher_motivation", "reliable_energy_signals"},
	domain.PhaseOvulatory:   {"ligament_laxity_peak", "acl_risk_elevated", "joint_focus_required"},
	domain.PhaseEarlyLuteal: {"progesterone_rising", "thermoregulation_affected", "pre_shift_window"},
	domain.PhaseLateLuteal:  {"cortisol_sensitivity_high", "sleep_disruption_risk", "higher_skip_cost"},
}

// ============================================================================
// MODALITY NOUN — token swap for banner title
// ============================================================================

var modalityNoun = map[domain.Modality]string{
	domain.ModalityStrength: "strength session",
	domain.ModalityHIIT:     "HIIT session",
	domain.ModalityPilates:  "Pilates session",
	domain.ModalityCardio:   "cardio session",
}

// Strength and HIIT "convert to strength", Pilates and Cardio "convert to adaptation"
var modalityOutcome = map[domain.Modality]string{
	domain.ModalityStrength: "strength",
	domain.ModalityHIIT:     "strength",
	domain.ModalityPilates:  "adaptation",
	domain.ModalityCardio:   "adaptation",
}

// ============================================================================
// HARDCODED VARIANTS
// ============================================================================

var variantO = domain.RecoveryBanner{
	Label:   "Recovery",
	Title:   "Fitness isn't built during training. It's built here.",
	Body:    "Training creates the stimulus. Recovery converts it to adaptation. VIV tracks both — because the work between sessions determines how much your sessions actually return.",
	Variant: "O",
}

var variantT = domain.RecoveryBanner{
	Title:   "Today's session starts the adaptation cycle.",
	Body:    "What you do after this session determines whether it converts. Protein within 2hrs, sleep tonight, and a nervous system reset tomorrow. The session is the stimulus — recovery is the result.",
	Variant: "T",
}

// ============================================================================
// BANNER INPUT — what the selector needs
// ============================================================================

// BannerInput contains everything needed to select and build a banner.
type BannerInput struct {
	Phase               domain.CyclePhase
	IsTrainingDay       bool
	TrainedYesterday    bool
	YesterdayModality   domain.Modality // only relevant if TrainedYesterday
	ConsecutiveRestDays int
	IsFirstWeek         bool
	Weekday             time.Weekday
}

// ============================================================================
// SELECTOR
// ============================================================================

// SelectBanner determines which recovery banner to show.
// Routing is pure logic — no LLM. Body copy comes from pre-generated library.
func SelectBanner(input BannerInput, library *BannerLibrary) domain.RecoveryBanner {
	dayName := strings.ToLower(input.Weekday.String())

	// Edge state 1: first week
	if input.IsFirstWeek {
		banner := variantO
		return banner
	}

	// Edge state 2: training day
	if input.IsTrainingDay {
		banner := variantT
		banner.Label = fmt.Sprintf("Today — %s · training day", capitalize(dayName))
		return banner
	}

	// Edge state 3: consecutive rest (no session yesterday)
	if !input.TrainedYesterday {
		body := ""
		if library != nil {
			body = library.GetNeutralBody(input.Phase, input.ConsecutiveRestDays)
		}
		if body == "" {
			body = "Multiple rest days allow deeper nervous system recovery. No session to convert right now — today's focus is maintaining the conditions that make your next training session more effective."
		}

		return domain.RecoveryBanner{
			Label:   fmt.Sprintf("Today — %s · rest day", capitalize(dayName)),
			Title:   "Your body is in active reset.",
			Body:    body,
			Variant: "N",
		}
	}

	// Primary state: rest day after session
	noun := modalityNoun[input.YesterdayModality]
	outcome := modalityOutcome[input.YesterdayModality]

	body := ""
	if library != nil {
		body = library.GetRecoveryBody(input.Phase, input.YesterdayModality, input.ConsecutiveRestDays)
	}
	if body == "" {
		body = "Your body is processing yesterday's training stimulus. Focus on protein, hydration, and sleep to maximize the adaptation window."
	}

	return domain.RecoveryBanner{
		Label:   fmt.Sprintf("Today — %s · rest day", capitalize(dayName)),
		Title:   fmt.Sprintf("Yesterday's %s is converting to %s right now.", noun, outcome),
		Body:    body,
		Variant: "R",
	}
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
