package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	openaiapi "github.com/sashabaranov/go-openai"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

// ============================================================================
// TRAINING STRUCTURE GENERATOR — implements usecase.TrainingStructureGenerator
// ============================================================================

// TrainingStructureGenerator uses OpenAI to propose weekly training structures.
type TrainingStructureGenerator struct {
	client *OpenAIClient
}

func NewTrainingStructureGenerator(client *OpenAIClient) *TrainingStructureGenerator {
	return &TrainingStructureGenerator{client: client}
}

// GenerateWeekStructure calls the LLM to distribute sessions across 7 days.
// The LLM receives a pre-calculated budget and spacing constraints.
// It only decides WHICH DAY gets which session type — not the content.
func (g *TrainingStructureGenerator) GenerateWeekStructure(
	ctx context.Context,
	input usecase.StructureGenerationInput,
) (domain.WeekArrangement, usecase.TokenUsage, error) {

	systemPrompt := buildStructureSystemPrompt()
	userPrompt := buildStructureUserPrompt(input)

	msgs := []openaiapi.ChatCompletionMessage{
		{Role: openaiapi.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openaiapi.ChatMessageRoleUser, Content: userPrompt},
	}

	resp, err := g.client.Chat(ctx, msgs)
	if err != nil {
		return domain.WeekArrangement{}, usecase.TokenUsage{}, fmt.Errorf("openai call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return domain.WeekArrangement{}, usecase.TokenUsage{}, fmt.Errorf("openai returned 0 choices")
	}

	raw := sanitizeJSON(resp.Choices[0].Message.Content)

	arrangement, err := parseWeekStructure(raw)
	if err != nil {
		return domain.WeekArrangement{}, usecase.TokenUsage{}, fmt.Errorf("failed to parse structure: %w", err)
	}

	tokens := usecase.TokenUsage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		Model:            g.client.Model(),
	}

	return arrangement, tokens, nil
}

// ============================================================================
// PROMPT BUILDERS
// ============================================================================

func buildStructureSystemPrompt() string {
	return `You are VIV's training plan structure generator. Your ONLY job is to distribute training sessions across a 7-day week.

CRITICAL RULES:
1. Return ONLY valid JSON matching the exact schema below. No markdown, no explanation.
2. You receive a budget (how many sessions of each modality) — distribute them across 7 days.
3. Respect ALL spacing constraints provided.
4. Rest days have "session": null.
5. Every day must be present (monday through sunday).
6. The "muscle_group" for each session must vary — don't repeat the same muscle group on adjacent days.
7. For STRENGTH: choose from "upper", "lower", "full_body", "core".
8. For HIIT: always use "full_body".
9. For PILATES: choose from "core", "full_body".
10. For CARDIO: always use "none".

RESPONSE SCHEMA:
{
  "days": [
    {"weekday": "monday",    "session": {"modality": "STRENGTH", "muscle_group": "lower", "intensity": "moderate", "duration_minutes": 45}},
    {"weekday": "tuesday",   "session": null},
    {"weekday": "wednesday", "session": {"modality": "PILATES", "muscle_group": "core", "intensity": "low", "duration_minutes": 40}},
    {"weekday": "thursday",  "session": null},
    {"weekday": "friday",    "session": {"modality": "STRENGTH", "muscle_group": "upper", "intensity": "moderate", "duration_minutes": 45}},
    {"weekday": "saturday",  "session": null},
    {"weekday": "sunday",    "session": null}
  ]
}`
}

func buildStructureUserPrompt(input usecase.StructureGenerationInput) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Cycle phase: %s\n\n", input.Phase))

	b.WriteString("Session budget for this week:\n")
	for _, m := range input.Budget.Modalities {
		line := fmt.Sprintf("- %s: %d session(s)", m.Modality, m.Count)
		if m.IntensityOverride != nil {
			line += fmt.Sprintf(", intensity must be %s", *m.IntensityOverride)
		}
		if m.VivRecommended {
			line += " (VIV-recommended, user didn't select this)"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(fmt.Sprintf("\nSession duration: %d minutes each\n", input.Budget.DurationMinutes))
	b.WriteString(fmt.Sprintf("Total sessions: %d\n", input.Budget.TotalSessions))

	if input.ModalitySkip != "" {
		b.WriteString("\nUser preference for this week (from check-in):\n")
		for _, skip := range strings.Split(input.ModalitySkip, ",") {
			skip = strings.TrimSpace(skip)
			if strings.ToLower(skip) == "lighter" {
				b.WriteString("- User requested lighter sessions this week. Respect the intensity overrides above.\n")
			} else if skip != "" {
				b.WriteString(fmt.Sprintf("- User is skipping %s this week. Do NOT include it even if it fits the phase.\n", skip))
			}
		}
	}

	if len(input.Constraints) > 0 {
		b.WriteString("\nSpacing constraints (MUST respect):\n")
		for _, c := range input.Constraints {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	if input.RetryFeedback != "" {
		b.WriteString(fmt.Sprintf("\nPREVIOUS ATTEMPT FAILED VALIDATION:\n%s\nPlease fix these issues in your new proposal.\n", input.RetryFeedback))
	}

	return b.String()
}

// ============================================================================
// RESPONSE PARSER
// ============================================================================

// llmWeekResponse is the raw shape of the LLM's JSON response.
type llmWeekResponse struct {
	Days []llmDayResponse `json:"days"`
}

type llmDayResponse struct {
	Weekday string      `json:"weekday"`
	Session *llmSession `json:"session"`
}

type llmSession struct {
	Modality        string `json:"modality"`
	MuscleGroup     string `json:"muscle_group"`
	Intensity       string `json:"intensity"`
	DurationMinutes int    `json:"duration_minutes"`
}

// parseWeekStructure converts the raw LLM JSON into a domain.WeekArrangement.
func parseWeekStructure(raw string) (domain.WeekArrangement, error) {
	var resp llmWeekResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return domain.WeekArrangement{}, fmt.Errorf("json unmarshal: %w", err)
	}

	if len(resp.Days) != 7 {
		return domain.WeekArrangement{}, fmt.Errorf("expected 7 days, got %d", len(resp.Days))
	}

	weekdayMap := map[string]time.Weekday{
		"monday":    time.Monday,
		"tuesday":   time.Tuesday,
		"wednesday": time.Wednesday,
		"thursday":  time.Thursday,
		"friday":    time.Friday,
		"saturday":  time.Saturday,
		"sunday":    time.Sunday,
	}

	var arrangement domain.WeekArrangement
	arrangement.WeekStart = currentWeekStart()

	for i, d := range resp.Days {
		weekday, ok := weekdayMap[strings.ToLower(d.Weekday)]
		if !ok {
			return domain.WeekArrangement{}, fmt.Errorf("invalid weekday: %q", d.Weekday)
		}

		slot := domain.TrainingDaySlot{
			Weekday: weekday,
		}

		if d.Session != nil {
			session, err := mapSession(d.Session)
			if err != nil {
				return domain.WeekArrangement{}, fmt.Errorf("day %d (%s): %w", i, d.Weekday, err)
			}
			slot.Session = &session
		}

		arrangement.Days[i] = slot
	}

	return arrangement, nil
}

func mapSession(s *llmSession) (domain.TrainingSession, error) {
	modality := domain.Modality(strings.ToUpper(s.Modality))
	switch modality {
	case domain.ModalityStrength, domain.ModalityHIIT, domain.ModalityPilates, domain.ModalityCardio:
		// valid
	default:
		return domain.TrainingSession{}, fmt.Errorf("invalid modality: %q", s.Modality)
	}

	muscleGroup := domain.MuscleGroup(strings.ToLower(s.MuscleGroup))
	switch muscleGroup {
	case domain.MuscleGroupUpper, domain.MuscleGroupLower, domain.MuscleGroupFullBody,
		domain.MuscleGroupCore, domain.MuscleGroupNone:
		// valid
	default:
		return domain.TrainingSession{}, fmt.Errorf("invalid muscle_group: %q", s.MuscleGroup)
	}

	intensity := domain.Intensity(strings.ToLower(s.Intensity))
	switch intensity {
	case domain.IntensityLow, domain.IntensityModerate, domain.IntensityHigh:
		// valid
	default:
		return domain.TrainingSession{}, fmt.Errorf("invalid intensity: %q", s.Intensity)
	}

	return domain.TrainingSession{
		Modality:        modality,
		MuscleGroup:     muscleGroup,
		Intensity:       intensity,
		DurationMinutes: s.DurationMinutes,
	}, nil
}

// currentWeekStart returns the Monday of the current week.
func currentWeekStart() time.Time {
	now := time.Now()
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	monday := now.AddDate(0, 0, -int(weekday-time.Monday))
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}
