package domain

// ============================================================================
// INGREDIENT LIBRARY — components solved into a plate at runtime, replacing
// the whole-recipe library. See internal/content/nutrition/ingredient.schema.json
// and meal_template.schema.json for the authoring schema these mirror.
// ============================================================================

// IngredientRole determines which template slot an ingredient can fill.
// protein_source/carb_source/fat_source are the 3 unknowns the solver solves
// for; vegetable/fruit are added at a fixed amount, not solved.
type IngredientRole string

const (
	RoleProteinSource IngredientRole = "protein_source"
	RoleCarbSource    IngredientRole = "carb_source"
	RoleFatSource     IngredientRole = "fat_source"
	RoleVegetable     IngredientRole = "vegetable"
	RoleFruit         IngredientRole = "fruit"
)

// MacrosPer100g is the nutrition density used to build the solver's linear
// system — NOT a serving's macros.
type MacrosPer100g struct {
	Calories float64 `json:"calories"`
	ProteinG float64 `json:"protein_g"`
	CarbsG   float64 `json:"carbs_g"`
	FatG     float64 `json:"fat_g"`
}

// UnitHint lets the solver round a raw gram amount to a human-readable
// serving, e.g. 110g -> "~2 eggs" instead of a bare gram figure.
type UnitHint struct {
	Label string  `json:"label"`
	Grams float64 `json:"grams"`
}

// Ingredient is one component of the library — objective nutrition facts
// only. Cuisine/flavor identity lives on MealTemplate, not here: a chicken
// breast is the same ingredient whether it ends up in a Mediterranean or
// Asian plate.
type Ingredient struct {
	ID             string         `json:"id"`
	Version        int            `json:"version"`
	ContentStatus  string         `json:"content_status"`
	Role           IngredientRole `json:"role"`
	Name           string         `json:"name"`
	MacrosPer100g  MacrosPer100g  `json:"macros_per_100g"`
	Contains       []string       `json:"contains"`
	DigestionFlags []string       `json:"digestion_flags"`
	UnitHint       *UnitHint      `json:"unit_hint,omitempty"`
	MinGrams       float64        `json:"min_grams"`
	MaxGrams       float64        `json:"max_grams"`
	Author         *string        `json:"author"`
	CreatedAt      string         `json:"created_at"`
}

// TemplateSlot is one role to fill when composing a plate from this template.
type TemplateSlot struct {
	Role                   IngredientRole `json:"role"`
	FixedGrams             float64        `json:"fixed_grams,omitempty"`
	PreferredIngredientIDs []string       `json:"preferred_ingredient_ids,omitempty"`
}

// MealTemplate defines plate STRUCTURE (which roles to fill) and IDENTITY
// (cuisine, eating style) — never quantities. Quantities are solved per user
// against their real macro target.
type MealTemplate struct {
	ID              string            `json:"id"`
	Version         int               `json:"version"`
	ContentStatus   string            `json:"content_status"`
	MealType        string            `json:"meal_type"`
	Name            string            `json:"name"`
	Description     string            `json:"description,omitempty"`
	Slots           []TemplateSlot    `json:"slots"`
	Cuisine         string            `json:"cuisine,omitempty"`
	EatingStyleTags []string          `json:"eating_style_tags,omitempty"`
	PhaseNotes      map[string]string `json:"phase_notes,omitempty"`
	Author          *string           `json:"author"`
	CreatedAt       string            `json:"created_at"`
}
