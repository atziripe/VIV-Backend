package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openaiapi "github.com/sashabaranov/go-openai"

	"viv/internal/adapters/mealgen"
)

// CopyGenerator implements mealgen.CopyGenerator using OpenAI — writes
// appetizing names/summaries for already-solved plates. Never touches
// ingredients, amounts, or macros: those come from the deterministic solver
// and are correct and final by the time this runs. This call is never on
// the request path — see mealgen.AsyncCopyEnricher.
type CopyGenerator struct {
	client *OpenAIClient
}

func NewCopyGenerator(client *OpenAIClient) *CopyGenerator {
	return &CopyGenerator{client: client}
}

func (g *CopyGenerator) GenerateCopy(ctx context.Context, requests []mealgen.CopyRequest) ([]mealgen.CopyResult, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	msgs := []openaiapi.ChatCompletionMessage{
		{Role: openaiapi.ChatMessageRoleSystem, Content: copySystemPrompt()},
		{Role: openaiapi.ChatMessageRoleUser, Content: buildCopyUserPrompt(requests)},
	}

	resp, err := g.client.Chat(ctx, msgs)
	if err != nil {
		return nil, fmt.Errorf("openai call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned 0 choices")
	}

	raw := sanitizeJSON(resp.Choices[0].Message.Content)

	var results []mealgen.CopyResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return nil, fmt.Errorf("failed to parse copy response: %w", err)
	}

	return results, nil
}

func copySystemPrompt() string {
	return `You are VIV's meal copywriter. You receive a list of already-decided meals — a template name and exact ingredients with amounts — and write an appetizing name and a short summary for each.

CRITICAL RULES:
1. Return ONLY a valid JSON array: [{"key": "...", "name": "...", "summary": "..."}], one entry per input, using the exact same "key" value you were given.
2. NEVER change, add, remove, or imply different ingredients or amounts than what you were given — the meal is already decided, you are only writing copy for it.
3. "name" is a short, appetizing dish name (not a restatement of the ingredient list).
4. "summary" is one sentence describing the dish and, if useful, a light prep note — warm tone, concise.
5. Never use restriction, weight-loss, or diet-culture language ("guilt-free", "clean", "burn fat", "low-cal", calorie call-outs, etc.) — describe the food itself, not what it does to the body.
6. No markdown, no explanation outside the JSON array.`
}

func buildCopyUserPrompt(requests []mealgen.CopyRequest) string {
	var b strings.Builder
	b.WriteString("Write copy for these meals:\n\n")
	for _, r := range requests {
		b.WriteString(fmt.Sprintf("key: %s\ntemplate: %s\ningredients:\n", r.Key, r.TemplateName))
		for _, ing := range r.Ingredients {
			b.WriteString(fmt.Sprintf("- %s (%s)\n", ing.Name, ing.Approx))
		}
		b.WriteString("\n")
	}
	return b.String()
}
