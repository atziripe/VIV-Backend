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
// MEAL TEMPLATE LIBRARY — loads and serves domain.MealTemplate content.
// ============================================================================

type MealTemplateLibrary struct {
	templates []domain.MealTemplate
}

// LoadMealTemplateLibrary reads all *.json files under dir (recursive),
// skipping JSON Schema files and ingredient files, and parses the rest as
// domain.MealTemplate.
func LoadMealTemplateLibrary(dir string) (*MealTemplateLibrary, error) {
	var templates []domain.MealTemplate

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

		var t domain.MealTemplate
		if err := json.Unmarshal(data, &t); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		if t.MealType == "" {
			return nil // not a template file (e.g. an ingredient living in the same tree)
		}
		if t.ID == "" {
			return fmt.Errorf("template in %s has empty id", path)
		}

		templates = append(templates, t)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading meal template library from %s: %w", dir, err)
	}
	if len(templates) == 0 {
		return nil, fmt.Errorf("no meal templates found in %s", dir)
	}

	return &MealTemplateLibrary{templates: templates}, nil
}

func (l *MealTemplateLibrary) Count() int { return len(l.templates) }

// ForMealType returns templates matching the given meal type.
func (l *MealTemplateLibrary) ForMealType(mealType string) []domain.MealTemplate {
	var out []domain.MealTemplate
	for _, t := range l.templates {
		if t.MealType == mealType {
			out = append(out, t)
		}
	}
	return out
}
