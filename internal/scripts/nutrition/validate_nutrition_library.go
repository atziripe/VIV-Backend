package main

// validate_nutrition_library/main.go
//
// Script standalone para validar la librería de recetas base de nutrition
// (la librería pre-curada que reemplaza la generación en vivo por LLM,
// siguiendo el mismo patrón que internal/content/training/).
//
// Vive en su propio subpaquete (a diferencia de validate_library.go) porque
// dos archivos "package main" con func main() en el mismo directorio no
// compilan juntos — go build ./... fallaría.
//
// Uso:
//   go run ./internal/scripts/validate_nutrition_library <path-to-recipes-dir> [minOptions]
//
// Ejemplo:
//   go run ./internal/scripts/validate_nutrition_library ./internal/content/nutrition
//   go run ./internal/scripts/validate_nutrition_library ./internal/content/nutrition 3
//
// Qué hace:
//   1. Lee todos los *.json del directorio (recursivo) y valida el schema.
//   2. Valida que los tags (contains, digestion_flags, cuisine, etc.) usen
//      vocabulario válido.
//   3. Chequea IDs duplicados.
//   4. COBERTURA — para cada bucket (meal_type × protein_profile), simula
//      las combinaciones REALISTAS de restricciones + condiciones digestivas
//      que una usuaria puede marcar en onboarding, filtra el pool, y
//      reporta el peor caso (menos opciones sobrevivientes) por bucket.
//      Si el peor caso cae debajo de minOptions (default 3, porque el
//      generador actual pide "exactamente 3 opciones por slot"), es un
//      gap real de contenido — no un ejercicio teórico de 2^8 combos.
//
// Exit code 0 si todo OK, 1 si hay errores o gaps de cobertura.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ============================================================================
// SCHEMA — debe matchear el JSON que autoran las recetas base
// ============================================================================

type Recipe struct {
	ID              string            `json:"id"`
	Version         int               `json:"version"`
	ContentStatus   string            `json:"content_status"`
	MealType        string            `json:"meal_type"`       // breakfast | lunch | dinner | pre_training
	ProteinProfile  string            `json:"protein_profile"` // plant_based | mixed | animal_based
	Name            string            `json:"name"`
	Summary         string            `json:"summary"`
	Ingredients     []Ingredient      `json:"ingredients"`
	Macros          Macros            `json:"macros"`          // macros base — se escalan en runtime, no se re-autoran por banda
	Contains        []string          `json:"contains"`        // qué contiene: meat, fish, egg, dairy, gluten, caffeine, nuts, shellfish
	DigestionFlags  []string          `json:"digestion_flags"` // qué condiciones dispara: bloating, reflux, sensitive_stomach, constipation, ibs_like_sensitivity
	Cuisine         string            `json:"cuisine,omitempty"`
	EatingStyleTags []string          `json:"eating_style_tags,omitempty"` // simple_quick, home_cooked, eat_out_friendly, meal_prep_friendly
	PhaseNotes      map[string]string `json:"phase_notes,omitempty"`       // copy opcional — las macros NO cambian por fase
	Author          *string           `json:"author"`
	CreatedAt       string            `json:"created_at"`
}

type Ingredient struct {
	Name    string  `json:"name"`
	AmountG float64 `json:"amount_g"`
	Approx  string  `json:"approx"`
}

type Macros struct {
	Calories int     `json:"calories"`
	ProteinG float64 `json:"protein_g"`
	CarbsG   float64 `json:"carbs_g"`
	FatG     float64 `json:"fat_g"`
}

// ============================================================================
// VOCABULARIO VÁLIDO — mapeado 1:1 a las opciones reales de onboarding
// ============================================================================

var (
	validMealTypes = map[string]bool{
		"breakfast": true, "lunch": true, "dinner": true, "pre_training": true,
	}
	validProteinProfiles = map[string]bool{
		"plant_based": true, "mixed": true, "animal_based": true,
	}
	// contains: de "Any dietary restrictions or allergies?"
	validContainsTags = map[string]bool{
		"meat": true, "fish": true, "egg": true, "dairy": true,
		"gluten": true, "caffeine": true, "nuts": true, "shellfish": true,
	}
	// digestion_flags: de "Anything digestion-related we should account for?"
	validDigestionTags = map[string]bool{
		"bloating": true, "reflux": true, "sensitive_stomach": true,
		"constipation": true, "ibs_like_sensitivity": true,
	}
	// cuisine: de "Any cuisine you eat most?" (opcional)
	validCuisines = map[string]bool{
		"mediterranean": true, "asian": true, "latin": true,
		"american": true, "middle_eastern": true,
	}
	// eating_style_tags: de "What does your everyday eating look like?"
	validEatingStyleTags = map[string]bool{
		"simple_quick": true, "home_cooked": true,
		"eat_out_friendly": true, "meal_prep_friendly": true,
	}
	validContentStatus = map[string]bool{
		"draft_llm_generated": true, "draft_human_written": true,
		"reviewed": true, "approved": true,
	}
	validPhases = map[string]bool{
		"menstrual": true, "follicular": true, "ovulatory": true,
		"early_luteal": true, "late_luteal": true,
	}
)

// ============================================================================
// VALIDACIÓN DE UNA RECETA INDIVIDUAL
// ============================================================================

func validateRecipe(r Recipe) []string {
	var errs []string
	add := func(format string, args ...interface{}) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	if r.ID == "" {
		add("id is empty")
	}
	if r.Version < 1 {
		add("version must be >= 1, got %d", r.Version)
	}
	if !validContentStatus[r.ContentStatus] {
		add("invalid content_status: %q", r.ContentStatus)
	}
	if !validMealTypes[r.MealType] {
		add("invalid meal_type: %q", r.MealType)
	}
	if !validProteinProfiles[r.ProteinProfile] {
		add("invalid protein_profile: %q", r.ProteinProfile)
	}
	if r.Name == "" {
		add("name is empty")
	}

	if len(r.Ingredients) == 0 {
		add("ingredients is empty")
	}
	for i, ing := range r.Ingredients {
		if ing.Name == "" {
			add("ingredients[%d].name is empty", i)
		}
		if ing.AmountG <= 0 {
			add("ingredients[%d].amount_g must be > 0, got %v", i, ing.AmountG)
		}
	}

	if r.Macros.Calories <= 0 {
		add("macros.calories must be > 0, got %d", r.Macros.Calories)
	}
	if r.Macros.ProteinG < 0 || r.Macros.CarbsG < 0 || r.Macros.FatG < 0 {
		add("macros must be non-negative")
	}

	for _, tag := range r.Contains {
		if !validContainsTags[tag] {
			add("invalid contains tag: %q", tag)
		}
	}
	for _, tag := range r.DigestionFlags {
		if !validDigestionTags[tag] {
			add("invalid digestion_flags tag: %q", tag)
		}
	}
	if r.Cuisine != "" && !validCuisines[r.Cuisine] {
		add("invalid cuisine: %q", r.Cuisine)
	}
	for _, tag := range r.EatingStyleTags {
		if !validEatingStyleTags[tag] {
			add("invalid eating_style_tags tag: %q", tag)
		}
	}
	// phase_notes es copy opcional (las macros no cambian por fase, a
	// diferencia de training) — si están presentes, las keys deben ser válidas,
	// pero no exigimos cobertura de las 5 fases.
	for phase, note := range r.PhaseNotes {
		if !validPhases[phase] {
			add("invalid phase_notes key: %q", phase)
		} else if strings.TrimSpace(note) == "" {
			add("empty phase_note for %q", phase)
		}
	}

	if r.CreatedAt == "" {
		add("created_at is empty")
	}

	return errs
}

// ============================================================================
// COBERTURA — combinaciones realistas de restricciones + digestión
// ============================================================================
// No simulamos las 2^8 combinaciones teóricas de restricciones — simulamos
// las que una usuaria real marcaría juntas. Esto es lo que separa este
// checker de "cobertura de archivo" (como training) de un check que de
// verdad valida selección con filtros combinados.

var restrictionCombos = [][]string{
	{},
	{"meat"},
	{"fish"},
	{"meat", "fish"},                 // vegetariana
	{"meat", "fish", "dairy", "egg"}, // vegana
	{"dairy"},
	{"egg"},
	{"dairy", "egg"},
	{"gluten"},
	{"gluten", "dairy"}, // paleo-ish
	{"nuts"},
	{"shellfish"},
	{"caffeine"},
	{"dairy", "gluten", "egg"},
}

var digestionCombos = [][]string{
	{},
	{"bloating"},
	{"reflux"},
	{"sensitive_stomach"},
	{"constipation"},
	{"ibs_like_sensitivity"},
	{"bloating", "ibs_like_sensitivity"}, // co-ocurrencia común
}

var mealTypes = []string{"breakfast", "lunch", "dinner", "pre_training"}
var proteinProfiles = []string{"plant_based", "mixed", "animal_based"}

// recommendedBucketSize es el tamaño objetivo por bucket para rotación
// semana a semana sin repetir — no es un mínimo de correctitud, es una
// meta de variedad. Ver conversación de diseño para el razonamiento.
var recommendedBucketSize = map[string]int{
	"breakfast|animal_based": 8, "breakfast|mixed": 7, "breakfast|plant_based": 8,
	"lunch|animal_based": 7, "lunch|mixed": 6, "lunch|plant_based": 6,
	"dinner|animal_based": 7, "dinner|mixed": 6, "dinner|plant_based": 6,
	"pre_training|animal_based": 5, "pre_training|mixed": 5, "pre_training|plant_based": 5,
}

func bucketKey(mealType, proteinProfile string) string {
	return mealType + "|" + proteinProfile
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

type coverageResult struct {
	Bucket             string
	TotalInBucket      int
	RecommendedTarget  int
	WorstCaseSurvivors int
	WorstCaseCombo     string
	CombosBelowMin     int
	CombosTested       int
}

// checkCoverage evalúa, por cada bucket (meal_type × protein_profile), el
// peor caso de sobrevivientes tras aplicar filtros de restricción +
// digestión simultáneos, y lo compara contra minOptions.
func checkCoverage(recipes []Recipe, minOptions int) []coverageResult {
	var results []coverageResult

	for _, mt := range mealTypes {
		for _, pp := range proteinProfiles {
			var bucket []Recipe
			for _, r := range recipes {
				if r.MealType == mt && r.ProteinProfile == pp {
					bucket = append(bucket, r)
				}
			}

			res := coverageResult{
				Bucket:             bucketKey(mt, pp),
				TotalInBucket:      len(bucket),
				RecommendedTarget:  recommendedBucketSize[bucketKey(mt, pp)],
				WorstCaseSurvivors: len(bucket), // arranca en el máximo posible (combo vacío)
				WorstCaseCombo:     "no restrictions",
			}

			for _, restr := range restrictionCombos {
				for _, dig := range digestionCombos {
					res.CombosTested++
					survivors := 0
					for _, r := range bucket {
						if hasOverlap(r.Contains, restr) {
							continue
						}
						if hasOverlap(r.DigestionFlags, dig) {
							continue
						}
						survivors++
					}
					if survivors < res.WorstCaseSurvivors {
						res.WorstCaseSurvivors = survivors
						res.WorstCaseCombo = describeCombo(restr, dig)
					}
					if survivors < minOptions {
						res.CombosBelowMin++
					}
				}
			}

			results = append(results, res)
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Bucket < results[j].Bucket })
	return results
}

func describeCombo(restr, dig []string) string {
	parts := []string{}
	if len(restr) == 0 {
		parts = append(parts, "no restrictions")
	} else {
		parts = append(parts, "avoids["+strings.Join(restr, ",")+"]")
	}
	if len(dig) > 0 {
		parts = append(parts, "digestion["+strings.Join(dig, ",")+"]")
	}
	return strings.Join(parts, " + ")
}

// ============================================================================
// MAIN
// ============================================================================

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run nutrition/validate_nutrition_library.go <path-to-recipes-dir> [minOptions]")
		os.Exit(2)
	}

	dir := os.Args[1]

	minOptions := 3
	if len(os.Args) >= 3 {
		v, err := strconv.Atoi(os.Args[2])
		if err != nil || v < 1 {
			fmt.Printf("invalid minOptions %q, must be a positive integer\n", os.Args[2])
			os.Exit(2)
		}
		minOptions = v
	}

	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		fmt.Printf("ERROR walking directory: %v\n", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Printf("ERROR: no .json files found in %s\n", dir)
		os.Exit(1)
	}

	fmt.Printf("Found %d JSON files in %s\n\n", len(files), dir)

	var recipes []Recipe
	seenIDs := map[string]string{} // id -> file path (para detectar duplicados)
	totalErrors := 0
	filesWithErrors := 0

	for _, path := range files {
		rel, _ := filepath.Rel(dir, path)

		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("✗ %s — read error: %v\n", rel, err)
			totalErrors++
			filesWithErrors++
			continue
		}

		var r Recipe
		if err := json.Unmarshal(data, &r); err != nil {
			fmt.Printf("✗ %s — JSON parse error: %v\n", rel, err)
			totalErrors++
			filesWithErrors++
			continue
		}

		errs := validateRecipe(r)

		if prev, dup := seenIDs[r.ID]; dup && r.ID != "" {
			errs = append(errs, fmt.Sprintf("duplicate id %q (also in %s)", r.ID, prev))
		} else if r.ID != "" {
			seenIDs[r.ID] = rel
		}

		if len(errs) > 0 {
			fmt.Printf("✗ %s\n", rel)
			for _, e := range errs {
				fmt.Printf("    - %s\n", e)
			}
			totalErrors += len(errs)
			filesWithErrors++
		} else {
			fmt.Printf("✓ %s  [%s/%s]\n", rel, r.MealType, r.ProteinProfile)
		}

		recipes = append(recipes, r)
	}

	fmt.Println()
	fmt.Printf("=== COVERAGE CHECK (min %d options must survive realistic filter combos) ===\n", minOptions)

	coverage := checkCoverage(recipes, minOptions)
	coverageGaps := 0

	for _, res := range coverage {
		status := "✓"
		if res.WorstCaseSurvivors < minOptions {
			status = "✗"
			coverageGaps++
		}
		targetNote := ""
		if res.RecommendedTarget > 0 && res.TotalInBucket < res.RecommendedTarget {
			targetNote = fmt.Sprintf("  (below variety target: %d/%d)", res.TotalInBucket, res.RecommendedTarget)
		}
		fmt.Printf("%s %-28s total=%-3d worst_case=%-3d [%s]  failing_combos=%d/%d%s\n",
			status, res.Bucket, res.TotalInBucket, res.WorstCaseSurvivors, res.WorstCaseCombo,
			res.CombosBelowMin, res.CombosTested, targetNote)
	}

	fmt.Println()
	fmt.Println("=== SUMMARY ===")
	fmt.Printf("Files processed:    %d\n", len(files))
	fmt.Printf("Files OK:           %d\n", len(files)-filesWithErrors)
	fmt.Printf("Files with errors:  %d\n", filesWithErrors)
	fmt.Printf("Schema errors:      %d\n", totalErrors)
	fmt.Printf("Buckets with coverage gaps: %d / %d\n", coverageGaps, len(coverage))

	if totalErrors > 0 || coverageGaps > 0 {
		os.Exit(1)
	}
	fmt.Println("\n✓ Nutrition library validation passed")
}
