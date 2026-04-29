package recovery

import "viv/internal/core/domain"

// ============================================================================
// MOVE SELECTOR — deterministic, no LLM
// ============================================================================
// Selects 2-3 move IDs based on phase + session_type + consecutive_rest_days.
// Pure logic, fully testable.

// SelectMoves returns the move IDs for today's recovery.
// Returns empty slice for training days (Variant T handles those).
func SelectMoves(phase domain.CyclePhase, sessionType domain.Modality, consecutiveRest int) []string {
	var selected []string

	// ── SLOT 1 — Phase priority move (always present) ────────────
	slot1 := phaseSlot1(phase)
	selected = append(selected, slot1)

	// ── SLOT 2 — Session type move ───────────────────────────────
	if consecutiveRest >= 2 {
		// Multiple rest days → gentle movement over session-specific recovery
		selected = append(selected, "J3")
	} else {
		slot2 := sessionSlot2(sessionType)
		selected = append(selected, slot2)
	}

	// ── SLOT 3 — Phase-specific extras (late luteal + menstrual only) ──
	if phase == domain.PhaseLateLuteal {
		selected = append(selected, "F1") // protein target
	}
	if phase == domain.PhaseMenstrual {
		selected = append(selected, "H4") // electrolyte protocol
	}

	return selected
}

// phaseSlot1 returns the phase-priority move for slot 1.
func phaseSlot1(phase domain.CyclePhase) string {
	switch phase {
	case domain.PhaseMenstrual:
		return "F3" // iron-rich meal
	case domain.PhaseFollicular:
		return "S1" // sleep duration
	case domain.PhaseOvulatory:
		return "J1" // joint mobility
	case domain.PhaseEarlyLuteal:
		return "S1" // sleep duration
	case domain.PhaseLateLuteal:
		return "S2" // sleep hygiene — luteal variant
	default:
		return "S1"
	}
}

// sessionSlot2 returns the session-type recovery move for slot 2.
func sessionSlot2(sessionType domain.Modality) string {
	switch sessionType {
	case domain.ModalityStrength:
		return "N1" // nervous system reset
	case domain.ModalityHIIT:
		return "F4" // glycogen replenishment
	case domain.ModalityPilates:
		return "H1" // scheduled hydration
	case domain.ModalityCardio:
		return "N2" // 10 min walk
	default:
		return "N2" // no session yesterday — default to walk
	}
}
