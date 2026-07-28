package nutrition

import "viv/internal/core/domain"

// ============================================================================
// SLOT TARGET DISTRIBUTION — splits a day's total macro target across its
// meal slots. Ports the percentages that used to live only as free-text
// instructions in meal_generator.go's prompt ("Breakfast: ~30% of daily
// carbs...") into explicit, testable Go — the solver needs a per-slot
// target, not a whole-day one.
// ============================================================================

// preTrainingFixedTarget is the fixed allocation for pre_training/
// post_training slots — the midpoint of the existing spec's range
// (200-250 kcal, 12-18g protein, 25-35g carbs, <=8g fat), carved out of the
// day's total before splitting the rest across main meals.
var preTrainingFixedTarget = domain.DayMacros{Calories: 225, ProteinG: 15, CarbsG: 30, FatG: 6}

var mainSlotCarbWeight = map[string]float64{"breakfast": 0.30, "lunch": 0.35, "dinner": 0.35}
var mainSlotFatWeight = map[string]float64{"breakfast": 0.25, "lunch": 0.30, "dinner": 0.35}

// DistributeSlotTargets splits dayTotal across slotTypes. pre_training/
// post_training get the fixed allocation above; protein on the remaining
// main slots (breakfast/lunch/dinner) splits evenly, carbs/fat split by
// weight — renormalized to whichever of the 3 are actually present, so a
// day missing lunch (e.g. midday-training days, which get post_training
// instead) still sums correctly.
func DistributeSlotTargets(dayTotal domain.DayMacros, slotTypes []string) map[string]domain.DayMacros {
	result := make(map[string]domain.DayMacros, len(slotTypes))

	remaining := dayTotal
	var mainSlots []string
	for _, s := range slotTypes {
		if s == "pre_training" || s == "post_training" {
			result[s] = preTrainingFixedTarget
			remaining.Calories -= preTrainingFixedTarget.Calories
			remaining.ProteinG -= preTrainingFixedTarget.ProteinG
			remaining.CarbsG -= preTrainingFixedTarget.CarbsG
			remaining.FatG -= preTrainingFixedTarget.FatG
		} else {
			mainSlots = append(mainSlots, s)
		}
	}

	if len(mainSlots) == 0 {
		return result
	}

	carbWeightSum, fatWeightSum := 0.0, 0.0
	for _, s := range mainSlots {
		carbWeightSum += mainSlotCarbWeight[s]
		fatWeightSum += mainSlotFatWeight[s]
	}

	proteinEach := remaining.ProteinG / float64(len(mainSlots))

	for _, s := range mainSlots {
		carbs := remaining.CarbsG * mainSlotCarbWeight[s] / carbWeightSum
		fat := remaining.FatG * mainSlotFatWeight[s] / fatWeightSum
		result[s] = domain.DayMacros{
			Calories: roundToInt(proteinEach*4 + carbs*4 + fat*9),
			ProteinG: roundTo1(proteinEach),
			CarbsG:   roundTo1(carbs),
			FatG:     roundTo1(fat),
		}
	}

	return result
}

func roundToInt(f float64) int {
	if f < 0 {
		return 0
	}
	return int(f + 0.5)
}

func roundTo1(f float64) float64 {
	if f < 0 {
		return 0
	}
	return float64(int(f*10+0.5)) / 10
}
