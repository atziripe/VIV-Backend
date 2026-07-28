package nutrition

import (
	"fmt"
	"math"

	"viv/internal/core/domain"
)

// ============================================================================
// PLATE SOLVER
// ============================================================================
// Composes a plate from a protein_source + carb_source + fat_source
// ingredient trio to hit a user's exact macro target, by solving a 3x3
// linear system instead of scaling a fixed recipe (one scale factor cannot
// hit 3 independent targets unless the base ratio already matches — see
// design discussion). vegetable/fruit slots are added at a fixed amount and
// are not solved; their macro contribution is subtracted from the target
// before solving.

// SolvedComponent is one ingredient's final amount in a composed plate.
type SolvedComponent struct {
	Ingredient domain.Ingredient
	RawGrams   float64 // solver output before clamping/rounding
	Grams      float64 // final amount used, after clamping + rounding
	Clamped    bool    // true if RawGrams fell outside [MinGrams, MaxGrams]
	Approx     string  // human-readable amount, derived from UnitHint
}

// SolveResult is a fully composed plate for one macro target.
type SolveResult struct {
	Components     []SolvedComponent
	TargetMacros   domain.DayMacros
	AchievedMacros domain.DayMacros // recomputed from final (clamped+rounded) grams
	MaxDeviation   float64          // largest relative |achieved-target|/target across the axes actually solved for
	// FatCeilingExceeded is only meaningful for SolvePlateTwoComponent,
	// where fat is a ceiling ("<= 8g") rather than a solved target — the
	// three-component solver always leaves this false since fat there is
	// one of the 3 solved axes and is already captured by MaxDeviation.
	FatCeilingExceeded bool
}

// FixedIngredient is a vegetable/fruit slot added at an authored fixed
// amount — not solved.
type FixedIngredient struct {
	Ingredient domain.Ingredient
	Grams      float64
}

// SolvePlate solves for grams of protein/carb/fat source needed to hit
// target, given fixed vegetable/fruit additions.
func SolvePlate(
	target domain.DayMacros,
	protein, carb, fat domain.Ingredient,
	fixed []FixedIngredient,
) (SolveResult, error) {
	if protein.Role != domain.RoleProteinSource {
		return SolveResult{}, fmt.Errorf("protein ingredient %q has role %q, want protein_source", protein.ID, protein.Role)
	}
	if carb.Role != domain.RoleCarbSource {
		return SolveResult{}, fmt.Errorf("carb ingredient %q has role %q, want carb_source", carb.ID, carb.Role)
	}
	if fat.Role != domain.RoleFatSource {
		return SolveResult{}, fmt.Errorf("fat ingredient %q has role %q, want fat_source", fat.ID, fat.Role)
	}

	// Fixed slots (vegetable/fruit) are consumed first; only what's left
	// gets solved for.
	remainingProtein := target.ProteinG
	remainingCarbs := target.CarbsG
	remainingFat := target.FatG
	for _, f := range fixed {
		factor := f.Grams / 100
		remainingProtein -= factor * f.Ingredient.MacrosPer100g.ProteinG
		remainingCarbs -= factor * f.Ingredient.MacrosPer100g.CarbsG
		remainingFat -= factor * f.Ingredient.MacrosPer100g.FatG
	}

	// Rows = macro dimension (protein/carbs/fat), columns = ingredient
	// (protein_source/carb_source/fat_source). Solving for hectograms
	// (grams/100) of each so macros_per_100g multiplies in directly.
	a := [3][3]float64{
		{protein.MacrosPer100g.ProteinG, carb.MacrosPer100g.ProteinG, fat.MacrosPer100g.ProteinG},
		{protein.MacrosPer100g.CarbsG, carb.MacrosPer100g.CarbsG, fat.MacrosPer100g.CarbsG},
		{protein.MacrosPer100g.FatG, carb.MacrosPer100g.FatG, fat.MacrosPer100g.FatG},
	}
	b := [3]float64{remainingProtein, remainingCarbs, remainingFat}

	det := det3(a)
	if math.Abs(det) < 1e-6 {
		return SolveResult{}, fmt.Errorf(
			"singular system: %s/%s/%s macro profiles are linearly dependent, cannot solve uniquely",
			protein.ID, carb.ID, fat.ID)
	}

	proteinHg := det3(replaceCol(a, 0, b)) / det
	carbHg := det3(replaceCol(a, 1, b)) / det
	fatHg := det3(replaceCol(a, 2, b)) / det

	components := []SolvedComponent{
		clampAndRound(protein, proteinHg*100),
		clampAndRound(carb, carbHg*100),
		clampAndRound(fat, fatHg*100),
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
		Components:     components,
		TargetMacros:   target,
		AchievedMacros: achieved,
		MaxDeviation:   maxDeviation(target, achieved),
	}, nil
}

func det3(m [3][3]float64) float64 {
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

func replaceCol(m [3][3]float64, col int, b [3]float64) [3][3]float64 {
	out := m
	for row := 0; row < 3; row++ {
		out[row][col] = b[row]
	}
	return out
}

// clampAndRound bounds a raw solved amount to the ingredient's realistic
// serving range, then snaps it to a human-readable unit.
func clampAndRound(ing domain.Ingredient, rawGrams float64) SolvedComponent {
	grams := rawGrams
	clamped := false
	if grams < ing.MinGrams {
		grams = ing.MinGrams
		clamped = true
	}
	if grams > ing.MaxGrams {
		grams = ing.MaxGrams
		clamped = true
	}

	// Rounding to a serving unit can push the amount back past MaxGrams
	// (e.g. 250g max, 33g units -> rounds to 264g) — re-clamp so max_grams
	// stays a hard bound even at the cost of an off-unit final amount.
	rounded := roundToUnit(ing, grams)
	if rounded > ing.MaxGrams {
		rounded = ing.MaxGrams
		clamped = true
	}
	if rounded < ing.MinGrams {
		rounded = ing.MinGrams
		clamped = true
	}

	return SolvedComponent{
		Ingredient: ing,
		RawGrams:   rawGrams,
		Grams:      rounded,
		Clamped:    clamped,
		Approx:     formatApprox(ing, rounded),
	}
}

// roundToUnit snaps grams to the nearest realistic serving increment.
// Falls back to the nearest 5g when the ingredient has no unit_hint.
func roundToUnit(ing domain.Ingredient, grams float64) float64 {
	step := 5.0
	if ing.UnitHint != nil && ing.UnitHint.Grams > 0 {
		step = ing.UnitHint.Grams
	}
	units := math.Round(grams / step)
	if units < 1 {
		units = 1
	}
	return units * step
}

func formatApprox(ing domain.Ingredient, grams float64) string {
	if ing.UnitHint == nil || ing.UnitHint.Grams <= 0 {
		return fmt.Sprintf("~%.0fg", grams)
	}
	units := grams / ing.UnitHint.Grams
	return fmt.Sprintf("~%.1f %s", units, ing.UnitHint.Label)
}

func sumMacros(components []SolvedComponent) domain.DayMacros {
	var calories, protein, carbs, fat float64
	for _, c := range components {
		factor := c.Grams / 100
		calories += factor * c.Ingredient.MacrosPer100g.Calories
		protein += factor * c.Ingredient.MacrosPer100g.ProteinG
		carbs += factor * c.Ingredient.MacrosPer100g.CarbsG
		fat += factor * c.Ingredient.MacrosPer100g.FatG
	}
	return domain.DayMacros{
		Calories: int(math.Round(calories)),
		ProteinG: math.Round(protein),
		CarbsG:   math.Round(carbs),
		FatG:     math.Round(fat),
	}
}

func maxDeviation(target, achieved domain.DayMacros) float64 {
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
	check(target.FatG, achieved.FatG)
	return dev
}
