package openai

import (
	"context"
	"fmt"
	"strings"

	openaiapi "github.com/sashabaranov/go-openai"

	"viv/internal/core/usecase"
)

// ============================================================================
// WEEKLY NOTE GENERATOR — implements usecase.WeeklyNoteGenerator (VIV-113)
// ============================================================================
//
// Same architectural shape as CopyGenerator (copy_generator.go): its only
// input is a small, plain-value DTO (usecase.WeeklyNoteInput) — there is
// no WeekDraft, DayPlan, or repository anywhere in this file, so there is
// no code path by which this call's output could feed back into plan
// generation.
type WeeklyNoteGenerator struct {
	client *OpenAIClient
}

func NewWeeklyNoteGenerator(client *OpenAIClient) *WeeklyNoteGenerator {
	return &WeeklyNoteGenerator{client: client}
}

func (g *WeeklyNoteGenerator) GenerateNote(ctx context.Context, input usecase.WeeklyNoteInput) (string, error) {
	msgs := []openaiapi.ChatCompletionMessage{
		{Role: openaiapi.ChatMessageRoleSystem, Content: weeklyNoteSystemPrompt()},
		{Role: openaiapi.ChatMessageRoleUser, Content: buildWeeklyNoteUserPrompt(input)},
	}

	resp, err := g.client.Chat(ctx, msgs)
	if err != nil {
		return "", fmt.Errorf("openai call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai returned 0 choices")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

func weeklyNoteSystemPrompt() string {
	return `You are VIV's weekly note writer. You are given a read-only summary of an already-finalized week — every session's activity type, intensity, impact, and muscle group is FINAL and already decided by the time you see it. You cannot change, question, or influence any of it. Your only job is to write one short, warm note (2-3 sentences) for today, reflecting today's specific session and the user's overall tone for her goal.

Use these as REFERENCE POINTS for tone per goal — NOT literal templates to copy. Your note must reflect the actual week you were given, never just restate an example:

- strength_muscle_tone: "You're well recovered today. Prioritise your progressive strength session and fuel it properly."
- body_composition_tone: "Keep today's strength session and support it with movement and protein across your meals."
- endurance_performance_tone: "Readiness is high. Today is a good opportunity for your interval or performance session."
- consistency_wellbeing_tone: "Today doesn't need to be perfect. A shorter, lower-intensity session keeps the week moving."

CRITICAL RULES:
1. Return ONLY the note text — no JSON, no markdown, no quotation marks, no explanation outside the note itself.
2. Never mention specific numbers, sets, reps, or durations — this is a tone-setting note, not a workout description.
3. Never contradict, reinterpret, or second-guess the session data you were given — describe it, don't redecide it.
4. Keep it to 2-3 sentences.
5. If today is a rest day, write a note that fits the tone but acknowledges rest, not a workout.`
}

func buildWeeklyNoteUserPrompt(input usecase.WeeklyNoteInput) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("This user's tone for her goal: %s\n", input.CopyTone))
	b.WriteString(fmt.Sprintf("Today is: %s\n\n", strings.Title(input.TodayWeekday)))

	b.WriteString("This week's finalized sessions (for context — write about TODAY specifically):\n")
	for _, d := range input.Days {
		marker := ""
		if strings.EqualFold(d.Weekday, input.TodayWeekday) {
			marker = " <-- TODAY"
		}
		if d.IsRestDay {
			b.WriteString(fmt.Sprintf("- %s: rest day%s\n", strings.Title(d.Weekday), marker))
			continue
		}
		b.WriteString(fmt.Sprintf("- %s: %s, intensity=%s, impact=%s, muscle_group=%s",
			strings.Title(d.Weekday), d.ActivityType, d.Intensity, d.Impact, d.MuscleGroup))
		if d.Substituted {
			b.WriteString(" (substituted from the original plan)")
		}
		b.WriteString(marker + "\n")
	}

	b.WriteString("\nWrite today's note now.")
	return b.String()
}
