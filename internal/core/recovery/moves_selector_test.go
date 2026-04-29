package recovery_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/recovery"
)

func TestSelectMoves_LateLutealAfterStrength(t *testing.T) {
	moves := recovery.SelectMoves(domain.PhaseLateLuteal, domain.ModalityStrength, 1)

	// Expect: S2 (sleep hygiene luteal), N1 (nervous system reset), F1 (protein target)
	if len(moves) != 3 {
		t.Fatalf("expected 3 moves, got %d: %v", len(moves), moves)
	}
	if moves[0] != "S2" {
		t.Errorf("slot 1: expected S2, got %s", moves[0])
	}
	if moves[1] != "N1" {
		t.Errorf("slot 2: expected N1, got %s", moves[1])
	}
	if moves[2] != "F1" {
		t.Errorf("slot 3: expected F1, got %s", moves[2])
	}
}

func TestSelectMoves_FollicularAfterHIIT(t *testing.T) {
	moves := recovery.SelectMoves(domain.PhaseFollicular, domain.ModalityHIIT, 1)

	// Expect: S1 (sleep duration), F4 (glycogen replenishment)
	if len(moves) != 2 {
		t.Fatalf("expected 2 moves, got %d: %v", len(moves), moves)
	}
	if moves[0] != "S1" {
		t.Errorf("slot 1: expected S1, got %s", moves[0])
	}
	if moves[1] != "F4" {
		t.Errorf("slot 2: expected F4, got %s", moves[1])
	}
}

func TestSelectMoves_OvulatoryAfterStrength(t *testing.T) {
	moves := recovery.SelectMoves(domain.PhaseOvulatory, domain.ModalityStrength, 1)

	// Expect: J1 (joint mobility), N1 (nervous system reset)
	if len(moves) != 2 {
		t.Fatalf("expected 2 moves, got %d: %v", len(moves), moves)
	}
	if moves[0] != "J1" {
		t.Errorf("slot 1: expected J1, got %s", moves[0])
	}
	if moves[1] != "N1" {
		t.Errorf("slot 2: expected N1, got %s", moves[1])
	}
}

func TestSelectMoves_MenstrualAfterPilates(t *testing.T) {
	moves := recovery.SelectMoves(domain.PhaseMenstrual, domain.ModalityPilates, 1)

	// Expect: F3 (iron-rich meal), H1 (scheduled hydration), H4 (electrolyte)
	if len(moves) != 3 {
		t.Fatalf("expected 3 moves, got %d: %v", len(moves), moves)
	}
	if moves[0] != "F3" {
		t.Errorf("slot 1: expected F3, got %s", moves[0])
	}
	if moves[1] != "H1" {
		t.Errorf("slot 2: expected H1, got %s", moves[1])
	}
	if moves[2] != "H4" {
		t.Errorf("slot 3: expected H4, got %s", moves[2])
	}
}

func TestSelectMoves_ConsecutiveRest_OverridesSlot2(t *testing.T) {
	// Day 2+ of rest → slot 2 becomes J3 regardless of session type
	moves := recovery.SelectMoves(domain.PhaseFollicular, domain.ModalityStrength, 2)

	if len(moves) != 2 {
		t.Fatalf("expected 2 moves, got %d: %v", len(moves), moves)
	}
	if moves[1] != "J3" {
		t.Errorf("slot 2: expected J3 (gentle movement) on consecutive rest, got %s", moves[1])
	}
}

func TestSelectMoves_ConsecutiveRest_Day3(t *testing.T) {
	moves := recovery.SelectMoves(domain.PhaseLateLuteal, domain.ModalityHIIT, 3)

	// Slot 2 should still be J3
	if moves[1] != "J3" {
		t.Errorf("slot 2: expected J3 on day 3 rest, got %s", moves[1])
	}
	// Slot 3 should still be F1 (late luteal)
	if moves[2] != "F1" {
		t.Errorf("slot 3: expected F1 for late luteal, got %s", moves[2])
	}
}

func TestSelectMoves_EarlyLutealAfterCardio(t *testing.T) {
	moves := recovery.SelectMoves(domain.PhaseEarlyLuteal, domain.ModalityCardio, 1)

	// Expect: S1 (sleep duration), N2 (10 min walk)
	if len(moves) != 2 {
		t.Fatalf("expected 2 moves, got %d: %v", len(moves), moves)
	}
	if moves[0] != "S1" {
		t.Errorf("slot 1: expected S1, got %s", moves[0])
	}
	if moves[1] != "N2" {
		t.Errorf("slot 2: expected N2, got %s", moves[1])
	}
}

func TestSelectMoves_NoSessionType_DefaultsToWalk(t *testing.T) {
	// Empty modality (consecutive rest, no session yesterday)
	moves := recovery.SelectMoves(domain.PhaseFollicular, "", 1)

	if moves[1] != "N2" {
		t.Errorf("slot 2: expected N2 (default walk) for no session, got %s", moves[1])
	}
}
