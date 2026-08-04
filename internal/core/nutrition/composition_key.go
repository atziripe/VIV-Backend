package nutrition

import (
	"fmt"
	"sort"
	"strings"

	"viv/internal/core/domain"
)

// CompositionKey returns a stable cache key for a solved plate: same
// template + same ingredients at the same (rounded) amounts should always
// get the same LLM-written copy, so this is what the copy cache indexes on
// (see internal/adapters/mealgen). Grams are rounded to the nearest 5g so
// trivial float differences between two otherwise-identical solves don't
// fragment the cache.
func CompositionKey(templateName string, ingredients []domain.MealIngredient) string {
	parts := make([]string, len(ingredients))
	for i, ing := range ingredients {
		rounded := int(ing.AmountG/5+0.5) * 5
		parts[i] = fmt.Sprintf("%s:%dg", ing.Name, rounded)
	}
	sort.Strings(parts)
	return templateName + "|" + strings.Join(parts, ",")
}
