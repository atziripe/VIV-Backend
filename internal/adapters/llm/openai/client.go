package openai

import (
	"context"
	"log"

	openai "github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client *openai.Client
	model  string
}

func NewOpenAIClient(apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		client: openai.NewClient(apiKey),
		model:  model,
	}
}

func (c *OpenAIClient) Model() string {
	return c.model
}

// Chat is the single entry point every real LLM call in this package goes
// through (CopyGenerator, TrainingStructureGenerator, WeeklyNoteGenerator,
// WeeklyScheduler) — logging token usage here, once, guarantees every one
// of them is covered without duplicating the same log line in each file,
// and covers any future caller automatically too.
//
// purpose identifies which feature made the call (e.g. "weekly_note",
// "weekly_scheduling") — every other structured log line in this codebase
// is feature-prefixed the same way (see [appcheck], [weeklynote],
// [scheduling], etc.), and per-feature token cost is the whole point:
// a single "[openai] ..." line with no purpose couldn't tell a cheap
// meal-copy call from an expensive scheduling one.
func (c *OpenAIClient) Chat(ctx context.Context, purpose string, messages []openai.ChatCompletionMessage) (openai.ChatCompletionResponse, error) {
	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:    c.model,
			Messages: messages,
		},
	)
	if err != nil {
		return resp, err
	}

	log.Printf("[openai] purpose=%s model=%s prompt_tokens=%d completion_tokens=%d total_tokens=%d",
		purpose, c.model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)

	return resp, nil
}
