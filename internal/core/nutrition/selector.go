package nutrition

import (
	"fmt"
	"math/rand"

	"viv/internal/core/domain"
)

// ============================================================================
// SELECTOR — picks WHICH ingredients to hand to the solver.
// ============================================================================
// CandidatesForRole only filters (by contains/digestion_flags); it doesn't
// rank. Naively taking candidates[0] can produce a bad plate: an
// ingredient's min_grams alone can already exceed a small target on one
// macro axis (chicken_breast's 60g floor is ~19g protein — more than a
// pre_training snack's whole protein budget), and "impure" sources (nut
// butters carry real protein/carbs alongside their fat) can skew the 3x3
// solution when picked blind. SelectBestPlate tries the feasible
// combinations and keeps the lowest-deviation one — same try-and-rank
// pattern as training/library.go's rankByPreferences.

// feasibilitySlack bounds how far a single ingredient's min_grams-implied
// amount on its own macro axis may exceed the target before it's excluded
// as a candidate for this target size.
const feasibilitySlack = 1.3

// SelectBestPlate tries combinations of protein/carb/fat candidates (already
// hard-filtered by CandidatesForRole) plus fixed vegetable/fruit slots, and
// returns the lowest-deviation solvable plate.
func SelectBestPlate(
	target domain.DayMacros,
	proteinCandidates, carbCandidates, fatCandidates []domain.Ingredient,
	fixed []FixedIngredient,
) (SolveResult, error) {
	proteinCandidates = feasibleFor(proteinCandidates, target.ProteinG, func(i domain.Ingredient) float64 {
		return i.MacrosPer100g.ProteinG
	})
	carbCandidates = feasibleFor(carbCandidates, target.CarbsG, func(i domain.Ingredient) float64 {
		return i.MacrosPer100g.CarbsG
	})
	fatCandidates = feasibleFor(fatCandidates, target.FatG, func(i domain.Ingredient) float64 {
		return i.MacrosPer100g.FatG
	})

	if len(proteinCandidates) == 0 || len(carbCandidates) == 0 || len(fatCandidates) == 0 {
		return SolveResult{}, fmt.Errorf(
			"no feasible combination for target %+v: %d protein, %d carb, %d fat candidates survive the min_grams feasibility check",
			target, len(proteinCandidates), len(carbCandidates), len(fatCandidates))
	}

	var best *SolveResult
	var bestErr error
	var ties []SolveResult

	for _, p := range proteinCandidates {
		for _, c := range carbCandidates {
			for _, f := range fatCandidates {
				result, err := SolvePlate(target, p, c, f, fixed)
				if err != nil {
					bestErr = err // remember in case nothing ever solves
					continue
				}
				switch {
				case best == nil || result.MaxDeviation < best.MaxDeviation-0.01:
					r := result
					best = &r
					ties = []SolveResult{result}
				case result.MaxDeviation <= best.MaxDeviation+0.01:
					ties = append(ties, result)
				}
			}
		}
	}

	if best == nil {
		if bestErr != nil {
			return SolveResult{}, fmt.Errorf("no combination of feasible candidates could be solved: %w", bestErr)
		}
		return SolveResult{}, fmt.Errorf("no combination of feasible candidates could be solved")
	}

	// Break near-ties randomly for variety across repeated calls, instead
	// of always returning the same plate.
	if len(ties) > 1 {
		return ties[rand.Intn(len(ties))], nil
	}
	return *best, nil
}

// SelectBestPlateTwoComponent is SelectBestPlate's counterpart for
// pre_training templates that carry no fat_source slot — see
// solver_two_component.go for why. Combinations that exceed fatCeiling are
// deprioritized (kept only if nothing else solves) rather than excluded
// outright, so a snack is still produced when every candidate runs a
// little over.
func SelectBestPlateTwoComponent(
	target domain.DayMacros,
	proteinCandidates, carbCandidates []domain.Ingredient,
	fixed []FixedIngredient,
	fatCeiling float64,
) (SolveResult, error) {
	proteinCandidates = feasibleFor(proteinCandidates, target.ProteinG, func(i domain.Ingredient) float64 {
		return i.MacrosPer100g.ProteinG
	})
	carbCandidates = feasibleFor(carbCandidates, target.CarbsG, func(i domain.Ingredient) float64 {
		return i.MacrosPer100g.CarbsG
	})

	if len(proteinCandidates) == 0 || len(carbCandidates) == 0 {
		return SolveResult{}, fmt.Errorf(
			"no feasible combination for target %+v: %d protein, %d carb candidates survive the min_grams feasibility check",
			target, len(proteinCandidates), len(carbCandidates))
	}

	var best *SolveResult
	var bestWithinCeiling *SolveResult
	var bestErr error
	var ties []SolveResult

	for _, p := range proteinCandidates {
		for _, c := range carbCandidates {
			result, err := SolvePlateTwoComponent(target, p, c, fixed, fatCeiling)
			if err != nil {
				bestErr = err
				continue
			}
			if best == nil || result.MaxDeviation < best.MaxDeviation-0.01 {
				r := result
				best = &r
				ties = []SolveResult{result}
			} else if result.MaxDeviation <= best.MaxDeviation+0.01 {
				ties = append(ties, result)
			}
			if !result.FatCeilingExceeded && (bestWithinCeiling == nil || result.MaxDeviation < bestWithinCeiling.MaxDeviation) {
				r := result
				bestWithinCeiling = &r
			}
		}
	}

	if best == nil {
		if bestErr != nil {
			return SolveResult{}, fmt.Errorf("no combination of feasible candidates could be solved: %w", bestErr)
		}
		return SolveResult{}, fmt.Errorf("no combination of feasible candidates could be solved")
	}

	// Prefer a plate that actually respects the fat ceiling, even if its
	// protein/carb deviation is slightly worse than the unconstrained best.
	if bestWithinCeiling != nil {
		return *bestWithinCeiling, nil
	}
	if len(ties) > 1 {
		return ties[rand.Intn(len(ties))], nil
	}
	return *best, nil
}

// feasibleFor excludes ingredients whose min_grams alone, on the given
// macro axis, would already exceed target by more than feasibilitySlack.
// If nothing passes (e.g. a very small target), falls back to the full
// candidate list rather than returning an empty pool — a wide miss the
// solver can still clamp is better than refusing to compose a plate.
func feasibleFor(candidates []domain.Ingredient, targetAxis float64, axis func(domain.Ingredient) float64) []domain.Ingredient {
	if targetAxis <= 0 {
		return candidates
	}
	var out []domain.Ingredient
	for _, ing := range candidates {
		floor := ing.MinGrams / 100 * axis(ing)
		if floor <= targetAxis*feasibilitySlack {
			out = append(out, ing)
		}
	}
	if len(out) == 0 {
		return candidates
	}
	return out
}
