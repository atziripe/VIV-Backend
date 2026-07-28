package nutrition

import (
	"fmt"
	"math"

	"viv/internal/core/domain"
)

// ============================================================================
// TWO-COMPONENT SOLVER — for meal types where fat is a ceiling, not a target.
// ============================================================================
// pre_training snacks are small enough that a mandatory fat_source slot
// doesn't fit: even the lowest-min_grams protein_source and fat_source
// combined incidentally exceed a ~6g fat budget before solving anything
// (see design discussion). The existing LLM-based generator already treats
// pre-training fat as a ceiling ("<= 8g"), not a percentage target — this
// mirrors that: solve a 2x2 system for protein_source + carb_source only,
// and let fat land wherever those two (plus any fixed slots) put it,
// flagging FatCeilingExceeded instead of folding fat into MaxDeviation.

// SolvePlateTwoComponent composes a plate from protein_source + carb_source
// only. fatCeiling is the maximum acceptable fat in the achieved plate —
// exceeding it doesn't error, it sets FatCeilingExceeded so the caller can
// try a different combination.
func SolvePlateTwoComponent(
	target domain.DayMacros,
	protein, carb domain.Ingredient,
	fixed []FixedIngredient,
	fatCeiling float64,
) (SolveResult, error) {
	if protein.Role != domain.RoleProteinSource {
		return SolveResult{}, fmt.Errorf("protein ingredient %q has role %q, want protein_source", protein.ID, protein.Role)
	}
	if carb.Role != domain.RoleCarbSource {
		return SolveResult{}, fmt.Errorf("carb ingredient %q has role %q, want carb_source", carb.ID, carb.Role)
	}

	remainingProtein := target.ProteinG
	remainingCarbs := target.CarbsG
	for _, f := range fixed {
		factor := f.Grams / 100
		remainingProtein -= factor * f.Ingredient.MacrosPer100g.ProteinG
		remainingCarbs -= factor * f.Ingredient.MacrosPer100g.CarbsG
	}

	// 2x2 system:
	//   pp*P + cp*C = remainingProtein
	//   pc*P + cc*C = remainingCarbs
	pp, pc := protein.MacrosPer100g.ProteinG, protein.MacrosPer100g.CarbsG
	cp, cc := carb.MacrosPer100g.ProteinG, carb.MacrosPer100g.CarbsG

	det := pp*cc - cp*pc
	if math.Abs(det) < 1e-6 {
		return SolveResult{}, fmt.Errorf(
			"singular system: %s/%s macro profiles are linearly dependent, cannot solve uniquely",
			protein.ID, carb.ID)
	}

	proteinHg := (remainingProtein*cc - cp*remainingCarbs) / det
	carbHg := (pp*remainingCarbs - remainingProtein*pc) / det

	components := []SolvedComponent{
		clampAndRound(protein, proteinHg*100),
		clampAndRound(carb, carbHg*100),
	}
	for _, f := range fixed {
		components = append(components, SolvedComponent{
			Ingredient: f.Ingredient,
			RawGrams:   f.Grams,
			Grams:      f.Grams,
			Approx:     formatApprox(f.Ingredient, f.Grams),
		})
	}

	achieved := sumMacros(components)

	return SolveResult{
		Components:         components,
		TargetMacros:       target,
		AchievedMacros:     achieved,
		MaxDeviation:       maxDeviationExcludingFat(target, achieved),
		FatCeilingExceeded: achieved.FatG > fatCeiling,
	}, nil
}

// maxDeviationExcludingFat is maxDeviation but skips the fat axis — fat
// isn't a target in the two-component path, it's a side effect checked
// separately via FatCeilingExceeded.
func maxDeviationExcludingFat(target, achieved domain.DayMacros) float64 {
	dev := 0.0
	check := func(t, a float64) {
		if t == 0 {
			return
		}
		if d := math.Abs(a-t) / t; d > dev {
			dev = d
		}
	}
	check(float64(target.Calories), float64(achieved.Calories))
	check(target.ProteinG, achieved.ProteinG)
	check(target.CarbsG, achieved.CarbsG)
	return dev
}
