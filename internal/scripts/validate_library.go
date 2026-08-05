package main

// validate_library.go
//
// Script standalone para validar la librería de sesiones de training.
//
// Uso:
//   go run validate_library.go <path-to-sessions-dir>
//
// Ejemplo:
//   go run validate_library.go ./internal/content/training
//
// Qué hace:
//   1. Lee todos los archivos *.json del directorio (recursivo).
//   2. Hace json.Unmarshal contra el schema esperado.
//   3. Valida que los campos críticos estén presentes y con valores válidos.
//   4. Valida cobertura mínima: que haya sesiones para cada combinación crítica.
//   5. Imprime un reporte por archivo + summary final.
//
// Exit code 0 si todo OK, 1 si hay errores.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ============================================================================
// SCHEMA — debe matchear exactamente el JSON generado por el LLM
// ============================================================================

type Session struct {
	ID              string            `json:"id"`
	Version         int               `json:"version"`
	ContentStatus   string            `json:"content_status"`
	Modality        string            `json:"modality"`
	MuscleGroup     string            `json:"muscle_group"`
	Intensity       string            `json:"intensity"`
	DurationMinutes int               `json:"duration_minutes"`
	SuitablePhases  []string          `json:"suitable_phases"`
	OptimizingFor   []string          `json:"optimizing_for"`
	RecoveryDemand  string            `json:"recovery_demand"`
	TimeCommitment  string            `json:"time_commitment"`
	ProgressionRole string            `json:"progression_role"`
	Content         Content           `json:"content"`
	PhaseNotes      map[string]string `json:"phase_notes"`
	Author          *string           `json:"author"`
	CreatedAt       string            `json:"created_at"`
}

type Content struct {
	Warmup        Block      `json:"warmup"`
	MainExercises []Exercise `json:"main_exercises"`
	Cooldown      Block      `json:"cooldown"`
}

type Block struct {
	DurationMinutes int      `json:"duration_minutes"`
	Description     string   `json:"description"`
	Movements       []string `json:"movements"`
}

type Exercise struct {
	Name         string `json:"name"`
	Sets         int    `json:"sets"`
	Reps         string `json:"reps"`
	RestSeconds  int    `json:"rest_seconds"`
	LoadGuidance string `json:"load_guidance"`
	FormCue      string `json:"form_cue"`
}

// ============================================================================
// VALORES VÁLIDOS — enums que el schema acepta
// ============================================================================

var (
	validModalities = map[string]bool{
		"STRENGTH": true, "HIIT": true, "PILATES": true, "CARDIO": true,
	}
	validMuscleGroups = map[string]bool{
		"upper": true, "lower": true, "full_body": true, "core": true, "none": true,
	}
	validIntensities = map[string]bool{
		"low": true, "moderate": true, "high": true,
	}
	validPhases = map[string]bool{
		"menstrual": true, "follicular": true, "ovulatory": true,
		"early_luteal": true, "late_luteal": true,
	}
	validOptimizingFor = map[string]bool{
		"strength and resilience":     true,
		"maintenance":                 true,
		"body recomposition":          true,
		"stress regulation":           true,
		"energy and consistency":      true,
		"performance and progression": true,
	}
	validRecoveryDemand  = map[string]bool{"low": true, "moderate": true, "high": true}
	validTimeCommitment  = map[string]bool{"low": true, "moderate": true, "high": true}
	validProgressionRole = map[string]bool{"build": true, "maintain": true, "recover": true}
	validContentStatus   = map[string]bool{
		"draft_llm_generated": true, "draft_human_written": true,
		"reviewed": true, "approved": true,
	}
)

// ============================================================================
// VALIDACIÓN DE UNA SESIÓN INDIVIDUAL
// ============================================================================

func validateSession(s Session, filename string) []string {
	var errs []string
	add := func(format string, args ...interface{}) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	// Campos obligatorios string
	if s.ID == "" {
		add("id is empty")
	}
	if s.Version < 1 {
		add("version must be >= 1, got %d", s.Version)
	}

	// Enums
	if !validContentStatus[s.ContentStatus] {
		add("invalid content_status: %q", s.ContentStatus)
	}
	if !validModalities[s.Modality] {
		add("invalid modality: %q", s.Modality)
	}
	if !validMuscleGroups[s.MuscleGroup] {
		add("invalid muscle_group: %q", s.MuscleGroup)
	}
	if !validIntensities[s.Intensity] {
		add("invalid intensity: %q", s.Intensity)
	}
	if !validRecoveryDemand[s.RecoveryDemand] {
		add("invalid recovery_demand: %q", s.RecoveryDemand)
	}
	if !validTimeCommitment[s.TimeCommitment] {
		add("invalid time_commitment: %q", s.TimeCommitment)
	}
	if !validProgressionRole[s.ProgressionRole] {
		add("invalid progression_role: %q", s.ProgressionRole)
	}

	// Duration
	if s.DurationMinutes <= 0 {
		add("duration_minutes must be > 0, got %d", s.DurationMinutes)
	}

	// Suitable phases
	if len(s.SuitablePhases) == 0 {
		add("suitable_phases is empty")
	}
	for _, p := range s.SuitablePhases {
		if !validPhases[p] {
			add("invalid suitable_phase: %q", p)
		}
	}

	// Optimizing for
	if len(s.OptimizingFor) == 0 {
		add("optimizing_for is empty")
	}
	if len(s.OptimizingFor) > 4 {
		add("optimizing_for has %d entries, max is 4", len(s.OptimizingFor))
	}
	for _, o := range s.OptimizingFor {
		if !validOptimizingFor[o] {
			add("invalid optimizing_for: %q", o)
		}
	}

	// Phase notes — debe haber una nota por cada una de las 5 fases
	requiredPhases := []string{"menstrual", "follicular", "ovulatory", "early_luteal", "late_luteal"}
	for _, p := range requiredPhases {
		note, ok := s.PhaseNotes[p]
		if !ok {
			add("missing phase_note for %q", p)
		} else if strings.TrimSpace(note) == "" {
			add("empty phase_note for %q", p)
		}
	}

	// Content — warmup
	if s.Content.Warmup.DurationMinutes <= 0 {
		add("content.warmup.duration_minutes must be > 0")
	}
	if len(s.Content.Warmup.Movements) == 0 {
		add("content.warmup.movements is empty")
	}

	// Content — main exercises
	if len(s.Content.MainExercises) == 0 {
		add("content.main_exercises is empty")
	}
	for i, ex := range s.Content.MainExercises {
		if ex.Name == "" {
			add("main_exercises[%d].name is empty", i)
		}
		if ex.Sets <= 0 {
			add("main_exercises[%d].sets must be > 0, got %d", i, ex.Sets)
		}
		if ex.Reps == "" {
			add("main_exercises[%d].reps is empty", i)
		}
		// rest_seconds puede ser 0 (algunos circuitos) — no validamos > 0
	}

	// Content — cooldown
	if s.Content.Cooldown.DurationMinutes <= 0 {
		add("content.cooldown.duration_minutes must be > 0")
	}
	if len(s.Content.Cooldown.Movements) == 0 {
		add("content.cooldown.movements is empty")
	}

	// Created at
	if s.CreatedAt == "" {
		add("created_at is empty")
	}

	return errs
}

// ============================================================================
// VALIDACIÓN DE COBERTURA — qué combinaciones esperamos tener
// ============================================================================

type expectedCombo struct {
	Modality    string
	MuscleGroup string
	Intensity   string
	MinCount    int
}

var expectedCoverage = []expectedCombo{
	// STRENGTH — 4 muscle groups × 2 intensities × 2 variants = 16
	{"STRENGTH", "upper", "moderate", 2},
	{"STRENGTH", "upper", "high", 2},
	{"STRENGTH", "lower", "moderate", 2},
	{"STRENGTH", "lower", "high", 2},
	{"STRENGTH", "full_body", "moderate", 2},
	{"STRENGTH", "full_body", "high", 2},
	{"STRENGTH", "core", "moderate", 2},
	{"STRENGTH", "core", "high", 2},
	// HIIT — 3 sessions
	{"HIIT", "full_body", "high", 3},
	// PILATES — 6 sessions
	{"PILATES", "core", "low", 2},
	{"PILATES", "full_body", "low", 4}, // 2 classical + 2 mobility
	// CARDIO — 4 sessions (2 low + 2 moderate)
	{"CARDIO", "none", "low", 2},
	{"CARDIO", "none", "moderate", 2},
}

func validateCoverage(sessions []Session) []string {
	var errs []string

	for _, exp := range expectedCoverage {
		count := 0
		for _, s := range sessions {
			if s.Modality == exp.Modality &&
				s.MuscleGroup == exp.MuscleGroup &&
				s.Intensity == exp.Intensity {
				count++
			}
		}
		if count < exp.MinCount {
			errs = append(errs, fmt.Sprintf(
				"coverage gap: %s/%s/%s — expected >= %d, got %d",
				exp.Modality, exp.MuscleGroup, exp.Intensity, exp.MinCount, count,
			))
		}
	}

	return errs
}

// ============================================================================
// MAIN
// ============================================================================

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run validate_library.go <path-to-sessions-dir>")
		os.Exit(2)
	}

	dir := os.Args[1]

	// Encontrar todos los .json del directorio (recursivo)
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

	// Validar cada archivo
	var sessions []Session
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

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			fmt.Printf("✗ %s — JSON parse error: %v\n", rel, err)
			totalErrors++
			filesWithErrors++
			continue
		}

		errs := validateSession(s, rel)
		if len(errs) > 0 {
			fmt.Printf("✗ %s\n", rel)
			for _, e := range errs {
				fmt.Printf("    - %s\n", e)
			}
			totalErrors += len(errs)
			filesWithErrors++
		} else {
			fmt.Printf("✓ %s  [%s/%s/%s]\n", rel, s.Modality, s.MuscleGroup, s.Intensity)
		}

		sessions = append(sessions, s)
	}

	// Validar cobertura
	fmt.Println()
	fmt.Println("=== COVERAGE CHECK ===")
	coverageErrs := validateCoverage(sessions)
	if len(coverageErrs) == 0 {
		fmt.Println("✓ All expected combinations covered")
	} else {
		for _, e := range coverageErrs {
			fmt.Printf("✗ %s\n", e)
		}
		totalErrors += len(coverageErrs)
	}

	// Summary
	fmt.Println()
	fmt.Println("=== SUMMARY ===")
	fmt.Printf("Files processed:  %d\n", len(files))
	fmt.Printf("Files OK:         %d\n", len(files)-filesWithErrors)
	fmt.Printf("Files with errors: %d\n", filesWithErrors)
	fmt.Printf("Coverage errors:  %d\n", len(coverageErrs))
	fmt.Printf("Total errors:     %d\n", totalErrors)

	if totalErrors > 0 {
		os.Exit(1)
	}
	fmt.Println("\n✓ Library validation passed")
}
