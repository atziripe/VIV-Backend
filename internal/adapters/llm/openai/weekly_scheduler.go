package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	openaiapi "github.com/sashabaranov/go-openai"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/scheduling"
	"viv/internal/core/usecase"
)

// ============================================================================
// WEEKLY SCHEDULER — implements usecase.SchedulingLayer (VIV-109)
// ============================================================================
//
// Explored the existing Layer 1 implementation (training_structure.go)
// before writing this: it asks the LLM to INVENT a week's modality/
// muscle_group/intensity/duration from a session budget — real freedom,
// fine for the old rule-engine pipeline it serves. This is deliberately
// narrower, because VIV-105/106 have already fully resolved every slot's
// content by the time scheduling runs: the LLM's only allowed output is
// which day each slot goes on. scheduling.ValidateAndApply is the hard
// backend gate that makes this true regardless of what the LLM's raw
// response says — never something left to prompt wording alone.
type WeeklyScheduler struct {
	client *OpenAIClient
}

func NewWeeklyScheduler(client *OpenAIClient) *WeeklyScheduler {
	return &WeeklyScheduler{client: client}
}

// Schedule implements usecase.SchedulingLayer. feedback is "" on a normal
// call; VIV-110's RuleEngineValidator passes a non-empty spacing-violation
// feedback string when re-calling this after the fact — that becomes the
// INITIAL prompt for this call (this call IS itself the one allowed retry
// at that level). Internally, Schedule may still perform its own separate
// single retry below if the LLM's response fails hard structural
// validation — a different, lower-level concern from VIV-110's spacing
// retry, so the two don't compound into more than one retry each.
func (s *WeeklyScheduler) Schedule(ctx context.Context, draft usecase.WeekDraft, feedback string) (usecase.WeekDraft, error) {
	var original []cascade.SlotAssignment
	for _, d := range draft.Days {
		if !d.IsRestDay {
			original = append(original, d.Assignment)
		}
	}
	restDayCount := 7 - len(original)

	// Nothing to schedule, or nothing to arrange relative to anything
	// else — no spacing constraint can even apply with 0-1 slots, so skip
	// the LLM call entirely.
	if len(original) < 2 {
		return draft, nil
	}

	systemPrompt := buildSchedulingSystemPrompt()
	userPrompt := buildSchedulingUserPrompt(original, feedback)

	days, err := s.requestAndValidate(ctx, systemPrompt, userPrompt, original, restDayCount)
	if err != nil {
		// One retry with concrete feedback — mirrors the old Layer 1
		// pipeline's single retry against rule-engine violations
		// (usecase.GenerateTrainingPlanUsecase).
		log.Printf("[scheduling] first attempt failed validation, retrying: %v", err)
		retryPrompt := buildSchedulingUserPrompt(original, err.Error())
		days, err = s.requestAndValidate(ctx, systemPrompt, retryPrompt, original, restDayCount)
		if err != nil {
			return usecase.WeekDraft{}, fmt.Errorf("scheduling: retry also failed: %w", err)
		}
	}

	if violations := scheduling.CheckSpacing(days); len(violations) > 0 {
		// Spacing is guidance the prompt asks the LLM to respect, not the
		// hard-block gate — that's the rule engine's job (VIV-110). Log
		// and proceed with the (still attribute-valid) placement rather
		// than hard-failing generation over it here.
		log.Printf("[scheduling] accepted placement still has spacing violations: %v", violations)
	}

	result := draft
	for i, d := range days {
		date := draft.StartDate.AddDate(0, 0, i)
		result.Days[i] = usecase.DayPlan{
			Date:       date,
			Weekday:    strings.ToLower(date.Weekday().String()),
			IsRestDay:  d.IsRestDay,
			Assignment: d.Assignment,
		}
	}
	return result, nil
}

func (s *WeeklyScheduler) requestAndValidate(
	ctx context.Context,
	systemPrompt, userPrompt string,
	original []cascade.SlotAssignment,
	restDayCount int,
) ([7]scheduling.DayPlacement, error) {
	msgs := []openaiapi.ChatCompletionMessage{
		{Role: openaiapi.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openaiapi.ChatMessageRoleUser, Content: userPrompt},
	}

	resp, err := s.client.Chat(ctx, "weekly_scheduling", msgs)
	if err != nil {
		return [7]scheduling.DayPlacement{}, fmt.Errorf("openai call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return [7]scheduling.DayPlacement{}, fmt.Errorf("openai returned 0 choices")
	}

	raw := sanitizeJSON(resp.Choices[0].Message.Content)

	placements, err := parseSchedulingResponse(raw)
	if err != nil {
		return [7]scheduling.DayPlacement{}, fmt.Errorf("failed to parse scheduling response: %w", err)
	}

	return scheduling.ValidateAndApply(original, restDayCount, placements)
}

// ============================================================================
// PROMPT BUILDERS
// ============================================================================

func buildSchedulingSystemPrompt() string {
	return `You are VIV's weekly session scheduler. You are given a list of ALREADY-DECIDED training sessions for this week — their activity type, intensity, impact, and muscle group are FINAL. You must not invent, drop, or change any of them.

YOUR ONLY JOB: decide which day of the week (Monday-Sunday) each session goes on. Every day not used by a session is a rest day.

CRITICAL RULES:
1. Return ONLY valid JSON matching the exact schema below. No markdown, no explanation.
2. Include exactly one placement per session listed — never more, never fewer.
3. For each placement, echo back its slot_index, activity_type, intensity, impact, and muscle_group EXACTLY as given, plus the weekday you chose for it. Do not alter any of the four echoed fields.
4. Respect the spacing constraints provided.

RESPONSE SCHEMA:
{
  "placements": [
    {"slot_index": 0, "weekday": "monday", "activity_type": "strength", "intensity": "M", "impact": "H", "muscle_group": "lower"},
    {"slot_index": 1, "weekday": "wednesday", "activity_type": "running", "intensity": "L", "impact": "H", "muscle_group": "lower"}
  ]
}`
}

func buildSchedulingUserPrompt(original []cascade.SlotAssignment, feedback string) string {
	var b strings.Builder

	b.WriteString("Sessions to place this week (content is FINAL, only choose a weekday for each):\n")
	for i, a := range original {
		cost := recoveryCostOf(a)
		b.WriteString(fmt.Sprintf(
			"[%d] activity_type=%s intensity=%s impact=%s muscle_group=%s recovery_cost=%d\n",
			i, a.ActivityType, a.Intensity, a.Impact, a.MuscleGroup, cost,
		))
	}

	b.WriteString("\nSpacing constraints (MUST respect):\n")
	b.WriteString("- No two consecutive days may both have impact=H.\n")
	b.WriteString("- Two sessions targeting the same muscle_group need a minimum number of rest days between them, based on the earlier session's recovery_cost:\n")
	for _, cost := range sortedRecoveryCostKeys() {
		days := scheduling.RestDaysRequired[cost]
		b.WriteString(fmt.Sprintf("  - recovery_cost %d -> at least %d rest day(s)\n", cost, days))
	}

	if feedback != "" {
		b.WriteString(fmt.Sprintf("\nPREVIOUS ATTEMPT FAILED VALIDATION:\n%s\nPlease fix this in your new proposal — remember, you may only choose weekdays, never change a session's attributes.\n", feedback))
	}

	return b.String()
}

// recoveryCostOf looks up a slot's RecoveryCost from VIV-101's taxonomy.
// Falls back to 0 (no mandatory gap) only for a slot the taxonomy somehow
// doesn't recognize — this can't happen for a slot that already passed
// through ResolveSlotConflict, which validates ActivityType/Intensity
// against activity.ByID itself; kept defensive rather than panicking on a
// prompt-building path.
func recoveryCostOf(a cascade.SlotAssignment) int {
	t, ok := activity.ByID(a.ActivityType)
	if !ok {
		return 0
	}
	cost, ok := t.RecoveryCost[a.Intensity]
	if !ok {
		return 0
	}
	return cost
}

func sortedRecoveryCostKeys() []int {
	keys := make([]int, 0, len(scheduling.RestDaysRequired))
	for k := range scheduling.RestDaysRequired {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// ============================================================================
// RESPONSE PARSER
// ============================================================================

type llmScheduleResponse struct {
	Placements []llmPlacement `json:"placements"`
}

type llmPlacement struct {
	SlotIndex    int                     `json:"slot_index"`
	Weekday      string                  `json:"weekday"`
	ActivityType activity.ID             `json:"activity_type"`
	Intensity    activity.IntensityLevel `json:"intensity"`
	Impact       activity.ImpactLevel    `json:"impact"`
	MuscleGroup  activity.MuscleGroup    `json:"muscle_group"`
}

func parseSchedulingResponse(raw string) ([]scheduling.Placement, error) {
	var resp llmScheduleResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	placements := make([]scheduling.Placement, len(resp.Placements))
	for i, p := range resp.Placements {
		placements[i] = scheduling.Placement{
			SlotIndex:    p.SlotIndex,
			Weekday:      p.Weekday,
			ActivityType: p.ActivityType,
			Intensity:    p.Intensity,
			Impact:       p.Impact,
			MuscleGroup:  p.MuscleGroup,
		}
	}
	return placements, nil
}
