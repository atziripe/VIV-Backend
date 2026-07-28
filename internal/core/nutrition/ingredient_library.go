package nutrition

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"viv/internal/core/domain"
)

// ============================================================================
// INGREDIENT LIBRARY — loads and serves domain.Ingredient content.
// Same load-once-at-boot, immutable, concurrency-safe pattern as
// training.Library.
// ============================================================================

type IngredientLibrary struct {
	ingredients []domain.Ingredient
}

// LoadIngredientLibrary reads all *.json files under dir (recursive),
// skipping the JSON Schema files themselves (ingredient.schema.json,
// meal_template.schema.json, recipe.schema.json), and parses the rest as
// domain.Ingredient.
func LoadIngredientLibrary(dir string) (*IngredientLibrary, error) {
	var ingredients []domain.Ingredient

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isSchemaFile(d.Name()) || !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		var i domain.Ingredient
		if err := json.Unmarshal(data, &i); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		if i.Role == "" {
			return nil // not an ingredient file (e.g. a meal template living in the same tree)
		}
		if i.ID == "" {
			return fmt.Errorf("ingredient in %s has empty id", path)
		}

		ingredients = append(ingredients, i)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading ingredient library from %s: %w", dir, err)
	}
	if len(ingredients) == 0 {
		return nil, fmt.Errorf("no ingredients found in %s", dir)
	}

	return &IngredientLibrary{ingredients: ingredients}, nil
}

func isSchemaFile(name string) bool {
	return strings.HasSuffix(name, ".schema.json")
}

func (l *IngredientLibrary) Count() int { return len(l.ingredients) }

// CandidatesForRole returns ingredients of the given role that don't
// contain anything in avoidContains or trigger anything in avoidDigestion —
// the same hard-filter semantics as before, just applied to ingredients
// instead of whole recipes.
func (l *IngredientLibrary) CandidatesForRole(role domain.IngredientRole, avoidContains, avoidDigestion []string) []domain.Ingredient {
	var out []domain.Ingredient
	for _, ing := range l.ingredients {
		if ing.Role != role {
			continue
		}
		if hasOverlap(ing.Contains, avoidContains) {
			continue
		}
		if hasOverlap(ing.DigestionFlags, avoidDigestion) {
			continue
		}
		out = append(out, ing)
	}
	return out
}

// ByID looks up a single ingredient — used to resolve
// TemplateSlot.PreferredIngredientIDs.
func (l *IngredientLibrary) ByID(id string) (domain.Ingredient, bool) {
	for _, ing := range l.ingredients {
		if ing.ID == id {
			return ing, true
		}
	}
	return domain.Ingredient{}, false
}

func hasOverlap(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, x := range a {
		set[x] = true
	}
	for _, x := range b {
		if set[x] {
			return true
		}
	}
	return false
}
