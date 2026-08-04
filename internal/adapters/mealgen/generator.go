// Package mealgen implements usecase.MealContentGenerator using the
// ingredient/template/solver pipeline in internal/core/nutrition instead of
// an LLM call — same role as internal/adapters/llm/openai's
// MealContentGenerator, just a different implementation of the same port.
//
// This lives in adapters, not core/nutrition, because internal/core/usecase
// already imports internal/core/nutrition (save_training_arrangement.go) —
// importing usecase back from core/nutrition would be an import cycle.
package mealgen

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"viv/internal/core/domain"
	"viv/internal/core/nutrition"
	"viv/internal/core/training"
	"viv/internal/core/usecase"
)

const optionsPerSlot = 3
const fatCeilingPrePost = 8.0 // matches meal_generator.go's "pre-training snack fat <= 8g" spec

// Generator implements usecase.MealContentGenerator.
type Generator struct {
	ingredients *nutrition.IngredientLibrary
	templates   *nutrition.MealTemplateLibrary
}

func New(ingredients *nutrition.IngredientLibrary, templates *nutrition.MealTemplateLibrary) *Generator {
	return &Generator{ingredients: ingredients, templates: templates}
}

func (g *Generator) GenerateWeekMeals(
	_ context.Context,
	input usecase.MealGenerationInput,
) ([]usecase.DayMealsOutput, usecase.TokenUsage, error) {
	avoidContains := nutrition.MapRestrictionsToContainsTags(input.DietRestrictions)
	avoidDigestion := nutrition.MapDigestiveConditionsToFlags(input.DigestiveConditions)
	proteinProfile := nutrition.MapProteinPreference(input.ProteinPreference)
	preferredStyle, preferredCuisine := nutrition.ParseEatingStyleAndCuisine(input.EatingStyle)

	var days []usecase.DayMealsOutput
	for _, day := range input.Days {
		slotTargets := nutrition.DistributeSlotTargets(day.Macros, day.MealSlots)
		timingBySlot := timingWindowsForDay(day, input.Phase)

		var meals []domain.MealSlot
		for _, slotType := range day.MealSlots {
			target, ok := slotTargets[slotType]
			if !ok {
				continue
			}
			slot, err := g.composeSlot(
				slotType, target, timingBySlot[slotType], input.Phase,
				avoidContains, avoidDigestion, proteinProfile, preferredCuisine, preferredStyle,
			)
			if err != nil {
				// A slot we can't compose (every candidate filtered out,
				// pool too thin) shouldn't fail the whole week — skip it
				// and let the gap show up in content QA rather than
				// blocking plan generation.
				continue
			}
			meals = append(meals, slot)
		}

		days = append(days, usecase.DayMealsOutput{Weekday: day.Weekday, Meals: meals})
	}

	return days, usecase.TokenUsage{Model: "ingredient_library_solver_v1"}, nil
}

func timingWindowsForDay(day usecase.MealDayInput, phase domain.CyclePhase) map[string]training.MealSlot {
	sessionTime := training.TrainingTimeOfDay(day.SessionTime)
	structure := training.ResolveMealStructure(day.IsTrainingDay, sessionTime, phase)
	out := make(map[string]training.MealSlot, len(structure.Slots))
	for _, s := range structure.Slots {
		out[string(s.Type)] = s
	}
	return out
}

func (g *Generator) composeSlot(
	slotType string,
	target domain.DayMacros,
	timing training.MealSlot,
	phase domain.CyclePhase,
	avoidContains, avoidDigestion []string,
	proteinProfile nutrition.ProteinProfile,
	preferredCuisine string,
	preferredStyle []string,
) (domain.MealSlot, error) {
	templateType := mapSlotTypeToTemplateMealType(slotType)
	templates := g.templates.ForMealType(templateType)
	if len(templates) == 0 {
		return domain.MealSlot{}, fmt.Errorf("no templates for meal_type %q (slot %q)", templateType, slotType)
	}
	templates = rankTemplates(templates, preferredCuisine, preferredStyle)

	baseProteinCands := applyProteinPreference(
		g.ingredients.CandidatesForRole(domain.RoleProteinSource, avoidContains, avoidDigestion),
		proteinProfile,
	)
	baseCarbCands := g.ingredients.CandidatesForRole(domain.RoleCarbSource, avoidContains, avoidDigestion)
	baseFatCands := g.ingredients.CandidatesForRole(domain.RoleFatSource, avoidContains, avoidDigestion)

	var options []domain.MealOption
	for _, tpl := range templates {
		if len(options) >= optionsPerSlot {
			break
		}

		// Narrow each role's pool to this template's cuisine-curated list
		// (when authored) before solving — this is what actually makes
		// "Latin"/"Asian"/etc. templates produce different ingredients
		// instead of all converging on the same objectively-best
		// combination for the target macros. filterByPreferred soft-falls
		// back to the unfiltered pool when the template has no preference
		// for that role, or when the preference would eliminate every
		// candidate (e.g. a restriction removed all of them).
		proteinCands := baseProteinCands
		carbCands := baseCarbCands
		var fixed []nutrition.FixedIngredient
		wantsFat := false
		for _, slot := range tpl.Slots {
			switch slot.Role {
			case domain.RoleProteinSource:
				proteinCands = filterByPreferred(baseProteinCands, slot.PreferredIngredientIDs)
			case domain.RoleCarbSource:
				carbCands = filterByPreferred(baseCarbCands, slot.PreferredIngredientIDs)
			case domain.RoleFatSource:
				wantsFat = true
			case domain.RoleVegetable, domain.RoleFruit:
				cands := filterByPreferred(
					g.ingredients.CandidatesForRole(slot.Role, avoidContains, avoidDigestion),
					slot.PreferredIngredientIDs,
				)
				if len(cands) > 0 {
					fixed = append(fixed, nutrition.FixedIngredient{Ingredient: cands[0], Grams: slot.FixedGrams})
				}
			}
		}

		var result nutrition.SolveResult
		var err error
		if wantsFat {
			fatCands := baseFatCands
			for _, slot := range tpl.Slots {
				if slot.Role == domain.RoleFatSource {
					fatCands = filterByPreferred(baseFatCands, slot.PreferredIngredientIDs)
				}
			}
			result, err = nutrition.SelectBestPlate(target, proteinCands, carbCands, fatCands, fixed)
		} else {
			result, err = nutrition.SelectBestPlateTwoComponent(target, proteinCands, carbCands, fixed, fatCeilingPrePost)
		}
		if err != nil {
			continue // this template didn't work out for this user — try the next one
		}

		options = append(options, toMealOption(tpl, result))
	}

	if len(options) == 0 {
		return domain.MealSlot{}, fmt.Errorf("no template produced a usable plate for slot %q", slotType)
	}

	var phaseNote *string
	for _, tpl := range templates {
		if note, ok := tpl.PhaseNotes[string(phase)]; ok && note != "" {
			n := note
			phaseNote = &n
			break
		}
	}

	return domain.MealSlot{
		MealName:     displaySlotName(slotType),
		TimingWindow: timing.TimingWindow,
		IsPrePost:    timing.IsPrePost,
		MacroTargets: target,
		Options:      options,
		PhaseNote:    phaseNote,
	}, nil
}

// mapSlotTypeToTemplateMealType routes post_training to the pre_training
// template pool — there's no dedicated post_training content yet, but
// pre_training's shape (protein+carb only, fat as a ceiling) fits the same
// "quick refeeding" need well enough as a stand-in.
func mapSlotTypeToTemplateMealType(slotType string) string {
	if slotType == "post_training" {
		return "pre_training"
	}
	return slotType
}

// filterByPreferred narrows candidates to a template-curated ingredient
// list (in the order preferredIDs lists them, so the first preferred
// ingredient wins ties/fixed-slot picks) when the template specifies one.
// Soft fallback to the unfiltered candidates when no preference is set, or
// when the preference matches nothing available (e.g. every preferred
// ingredient got excluded by a restriction) — a cuisine preference miss
// shouldn't block generation the way a hard restriction violation would.
func filterByPreferred(candidates []domain.Ingredient, preferredIDs []string) []domain.Ingredient {
	if len(preferredIDs) == 0 {
		return candidates
	}
	byID := make(map[string]domain.Ingredient, len(candidates))
	for _, c := range candidates {
		byID[c.ID] = c
	}
	var filtered []domain.Ingredient
	for _, id := range preferredIDs {
		if c, ok := byID[id]; ok {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) == 0 {
		return candidates
	}
	return filtered
}

// isAnimalSourced derives an animal/plant classification from the existing
// contains tags instead of a separate authored field — an ingredient tagged
// meat/fish/egg/dairy is animal-sourced, anything else (tofu, tempeh,
// lentils, plant protein powder, legumes...) is plant-based.
func isAnimalSourced(ing domain.Ingredient) bool {
	for _, tag := range ing.Contains {
		if tag == "meat" || tag == "fish" || tag == "shellfish" || tag == "egg" || tag == "dairy" {
			return true
		}
	}
	return false
}

// applyProteinPreference narrows the protein_source pool toward the user's
// stated preference. Falls back to the unfiltered pool when the preference
// would eliminate every candidate — a soft preference miss shouldn't block
// generation the way a hard restriction violation would.
func applyProteinPreference(candidates []domain.Ingredient, pref nutrition.ProteinProfile) []domain.Ingredient {
	if pref == nutrition.ProfileMixed {
		return candidates
	}
	var filtered []domain.Ingredient
	for _, c := range candidates {
		animal := isAnimalSourced(c)
		if (pref == nutrition.ProfileAnimalBased && animal) || (pref == nutrition.ProfilePlantBased && !animal) {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) == 0 {
		return candidates
	}
	return filtered
}

// rankTemplates stable-sorts so templates matching the user's cuisine/style
// preference come first — the 3 offered options are built from the first 3
// templates in this order, which also naturally gives cuisine variety
// across options instead of "3 variations of the same dish."
func rankTemplates(templates []domain.MealTemplate, preferredCuisine string, preferredStyle []string) []domain.MealTemplate {
	styleSet := make(map[string]bool, len(preferredStyle))
	for _, s := range preferredStyle {
		styleSet[s] = true
	}
	score := func(t domain.MealTemplate) int {
		s := 0
		if preferredCuisine != "" && t.Cuisine == preferredCuisine {
			s += 2
		}
		for _, tag := range t.EatingStyleTags {
			if styleSet[tag] {
				s++
			}
		}
		return s
	}
	out := make([]domain.MealTemplate, len(templates))
	copy(out, templates)
	sort.SliceStable(out, func(i, j int) bool { return score(out[i]) > score(out[j]) })
	return out
}

func toMealOption(tpl domain.MealTemplate, result nutrition.SolveResult) domain.MealOption {
	var ingredients []domain.MealIngredient
	var nameParts []string
	for _, c := range result.Components {
		ingredients = append(ingredients, domain.MealIngredient{
			Name:    c.Ingredient.Name,
			AmountG: c.Grams,
			Approx:  c.Approx,
		})
		nameParts = append(nameParts, c.Ingredient.Name)
	}
	return domain.MealOption{
		Name:        tpl.Name,
		Summary:     strings.Join(nameParts, ", "),
		Ingredients: ingredients,
		Macros:      result.AchievedMacros,
	}
}

func displaySlotName(slotType string) string {
	switch slotType {
	case "breakfast":
		return "Breakfast"
	case "lunch":
		return "Lunch"
	case "dinner":
		return "Dinner"
	case "pre_training":
		return "Pre-training"
	case "post_training":
		return "Post-training"
	default:
		return slotType
	}
}
