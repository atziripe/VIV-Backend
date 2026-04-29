package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openaiapi "github.com/sashabaranov/go-openai"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

// MealContentGenerator implements usecase.MealContentGenerator using OpenAI.
type MealContentGenerator struct {
	client *OpenAIClient
}

func NewMealContentGenerator(client *OpenAIClient) *MealContentGenerator {
	return &MealContentGenerator{client: client}
}

func (g *MealContentGenerator) GenerateWeekMeals(
	ctx context.Context,
	input usecase.MealGenerationInput,
) ([]usecase.DayMealsOutput, usecase.TokenUsage, error) {

	systemPrompt := buildMealSystemPrompt()
	userPrompt := buildMealUserPrompt(input)

	msgs := []openaiapi.ChatCompletionMessage{
		{Role: openaiapi.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openaiapi.ChatMessageRoleUser, Content: userPrompt},
	}

	resp, err := g.client.Chat(ctx, msgs)
	if err != nil {
		return nil, usecase.TokenUsage{}, fmt.Errorf("openai call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, usecase.TokenUsage{}, fmt.Errorf("openai returned 0 choices")
	}

	raw := sanitizeJSON(resp.Choices[0].Message.Content)

	// Parse the 2-template response
	var result struct {
		TrainingDay struct {
			Meals []domain.MealSlot `json:"meals"`
		} `json:"training_day"`
		RestDay struct {
			Meals []domain.MealSlot `json:"meals"`
		} `json:"rest_day"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, usecase.TokenUsage{}, fmt.Errorf("failed to parse meals: %w", err)
	}

	// Expand 2 templates into 7 days based on which days are training vs rest
	var days []usecase.DayMealsOutput
	for _, day := range input.Days {
		meals := result.RestDay.Meals
		if day.IsTrainingDay {
			meals = result.TrainingDay.Meals
		}
		days = append(days, usecase.DayMealsOutput{
			Weekday: day.Weekday,
			Meals:   meals,
		})
	}

	tokens := usecase.TokenUsage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		Model:            g.client.Model(),
	}

	return days, tokens, nil
}

func buildMealSystemPrompt() string {
	return `You are VIV's nutrition plan generator. You create personalized meal options for two day types: training day and rest day.

CRITICAL RULES:
1. Return ONLY valid JSON matching the exact schema below. No markdown, no explanation.
2. Generate exactly 2 day templates: "training_day" and "rest_day".
3. Training day has 4 meal slots: breakfast, pre_training, lunch, dinner.
4. Rest day has 3 meal slots: breakfast, lunch, dinner.
5. Each standard meal slot (breakfast, lunch, dinner) has exactly 3 options.
6. Pre-training snack has exactly 3 options.
7. Every ingredient must have amount_g as a positive number and an approx human-readable measure.
8. Meal option macros must approximately match the slot macro_targets (within ±15%).
9. NEVER suggest fasting, skipping meals, or raw eggs.
10. Total daily calories must be ≥ 1400.
11. Pre-training snack fat must be ≤ 8g per option.
12. Options within the same slot must be meaningfully different — not 3 variations of oatmeal.
13. At least one whole food protein source per main meal option (not protein powder alone).

DIETARY RULES:
- Respect ALL dietary restrictions strictly. If "no dairy" is listed, NO option may contain dairy.
- For plant-based protein preference, ensure every option has a complete protein source (combine legumes + grains if needed).
- For digestive issues like bloating or IBS, avoid high-FODMAP trigger foods (garlic, onion, large quantities of beans, cruciferous vegetables).

RESPONSE SCHEMA:
{
  "training_day": {
    "meals": [
      {
        "meal_name": "Breakfast",
        "timing_window": "7–9am",
        "macro_targets": {"calories": 500, "protein_g": 35, "carbs_g": 55, "fat_g": 18},
        "options": [
          {
            "name": "Overnight oats with berries",
            "summary": "oats, Greek yogurt, mixed berries, honey, chia seeds",
            "flag": null,
            "flag_reason": null,
            "ingredients": [
              {"name": "rolled oats", "amount_g": 60, "approx": "~2/3 cup"},
              {"name": "Greek yogurt", "amount_g": 150, "approx": "~2/3 cup"}
            ],
            "macros": {"calories": 480, "protein_g": 32, "carbs_g": 58, "fat_g": 16}
          },
          { "...option 2..." },
          { "...option 3..." }
        ],
        "phase_note": "Rising estrogen supports carb utilization — oats are ideal fuel."
      },
      {
        "meal_name": "Pre-training",
        "timing_window": "90min before session",
        "macro_targets": {"calories": 225, "protein_g": 15, "carbs_g": 30, "fat_g": 6},
        "options": [ "...3 options, fat ≤ 8g each..." ],
        "phase_note": "..."
      },
      { "...lunch..." },
      { "...dinner..." }
    ]
  },
  "rest_day": {
    "meals": [
      { "...breakfast with 3 options..." },
      { "...lunch with 3 options..." },
      { "...dinner with 3 options..." }
    ]
  }
}

PHASE-SPECIFIC MEAL NOTES:
- menstrual: iron-rich foods (red meat, spinach, lentils), anti-inflammatory (turmeric, ginger, fatty fish)
- follicular: higher carb tolerance, lean proteins, colorful vegetables
- ovulatory: lighter meals, raw vegetables OK, lean protein
- early_luteal: complex carbs, magnesium-rich foods (dark chocolate, nuts, seeds)
- late_luteal: blood sugar stabilizing (protein + complex carbs every meal), B6-rich foods

MACRO DISTRIBUTION ACROSS SLOTS:
- Protein: distribute roughly evenly across main meals
- Breakfast: ~30% of daily carbs, ~25% of daily fat
- Lunch: ~35% of daily carbs, ~30% of daily fat
- Dinner: ~35% of daily carbs, ~35% of daily fat
- Pre-training: 12-18g protein, 25-35g carbs, max 8g fat, 200-250 kcal

IMPORTANT: Generate unique, varied meals matching the specific macro targets, dietary preferences, and phase.`
}

func buildMealUserPrompt(input usecase.MealGenerationInput) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Cycle phase: %s\n\n", input.Phase))

	b.WriteString("Dietary profile:\n")
	if len(input.DietRestrictions) > 0 {
		b.WriteString(fmt.Sprintf("- Restrictions: %s\n", strings.Join(input.DietRestrictions, ", ")))
	} else {
		b.WriteString("- Restrictions: none\n")
	}
	b.WriteString(fmt.Sprintf("- Protein preference: %s\n", input.ProteinPreference))
	if len(input.DigestiveConditions) > 0 {
		b.WriteString(fmt.Sprintf("- Digestive considerations: %s\n", strings.Join(input.DigestiveConditions, ", ")))
	}
	if input.AppetiteStatus != "" {
		b.WriteString(fmt.Sprintf("- Current appetite: %s\n", input.AppetiteStatus))
	}

	b.WriteString("\nMacro targets:\n")
	b.WriteString(fmt.Sprintf("- Training day: %d kcal, %.0fg protein, %.0fg carbs, %.0fg fat\n",
		input.Targets.TrainingDay.Calories, input.Targets.TrainingDay.ProteinG,
		input.Targets.TrainingDay.CarbsG, input.Targets.TrainingDay.FatG))
	b.WriteString(fmt.Sprintf("- Rest day: %d kcal, %.0fg protein, %.0fg carbs, %.0fg fat\n",
		input.Targets.RestDay.Calories, input.Targets.RestDay.ProteinG,
		input.Targets.RestDay.CarbsG, input.Targets.RestDay.FatG))

	b.WriteString(fmt.Sprintf("\nSession time: %s\n", input.Days[0].SessionTime))

	b.WriteString("\nGenerate exactly 3 options per meal slot. Return valid JSON only.")

	return b.String()
}
