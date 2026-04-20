package domain

import "time"

type Modality string

const (
	ModalityStrength Modality = "STRENGTH"
	ModalityHIIT     Modality = "HIIT"
	ModalityPilates  Modality = "PILATES"
	ModalityCardio   Modality = "CARDIO"
)

type MuscleGroup string

const (
	MuscleGroupUpper    MuscleGroup = "upper"
	MuscleGroupLower    MuscleGroup = "lower"
	MuscleGroupFullBody MuscleGroup = "full_body"
	MuscleGroupCore     MuscleGroup = "core"
	MuscleGroupNone     MuscleGroup = "none"
)

type Intensity string

const (
	IntensityLow      Intensity = "low"
	IntensityModerate Intensity = "moderate"
	IntensityHigh     Intensity = "high"
)

type CyclePhase string

const (
	PhaseMenstrual   CyclePhase = "menstrual"
	PhaseFollicular  CyclePhase = "follicular"
	PhaseOvulatory   CyclePhase = "ovulatory"
	PhaseEarlyLuteal CyclePhase = "early_luteal"
	PhaseLateLuteal  CyclePhase = "late_luteal"
)

// ============================================================================
// TIPOS ESTRUCTURALES — lo que el engine valida
// ============================================================================
// NOTA: TrainingSession es deliberadamente LIGERA. Contiene solo los campos
// que las reglas necesitan para validar. El contenido completo (ejercicios,
// sets, reps) vive en la librería y se hidrata DESPUÉS de validar.
//
// Esto es una decisión de diseño clave: el engine opera sobre una proyección
// del plan, no sobre el plan completo. Mantiene las reglas simples y el engine
// trivial de testear.

// TrainingSession representa una sesión de entrenamiento en su forma mínima
// validable por el engine. No contiene ejercicios ni contenido detallado.
//
// Nombre prefixeado con "Training" para no colisionar con otros conceptos
// de "session" en el dominio de VIV (nutrición, recovery, etc.).
type TrainingSession struct {
	Modality        Modality    `json:"modality"`
	MuscleGroup     MuscleGroup `json:"muscle_group"`
	Intensity       Intensity   `json:"intensity"`
	DurationMinutes int         `json:"duration_minutes"`
}

// TrainingDaySlot representa un día de la semana con opcionalmente una sesión.
// Session == nil significa rest day.
type TrainingDaySlot struct {
	Weekday time.Weekday     `json:"weekday"`
	Session *TrainingSession `json:"session,omitempty"`
}

// WeekArrangement es la estructura semanal completa que el engine valida:
// 7 días consecutivos, cada uno con o sin sesión.
//
// Usamos un array [7] en vez de slice []  para garantizar a nivel de tipo
// que siempre hay exactamente 7 días. Menos bugs, más explícito.
type WeekArrangement struct {
	WeekStart time.Time          `json:"week_start"`
	Days      [7]TrainingDaySlot `json:"days"`
}

// ============================================================================
// TIPOS DE LA LIBRERÍA DE TRAINING
// ============================================================================
// Estos tipos representan una sesión COMPLETA como vive en los archivos JSON
// de la librería. Incluyen contenido (ejercicios, warmup, cooldown),
// metadata de clasificación, y phase notes.
//
// Diferencia clave con TrainingSession:
//   - TrainingSession = lo que el engine valida (4 campos)
//   - LibrarySession  = lo que la librería almacena (todo el contenido)
//
// El selector de Fase 2 busca en LibrarySessions y devuelve la que matchea.
// El engine nunca ve estos tipos.

// LibrarySession es una sesión completa de la librería de training.

type LibrarySession struct {
	ID              string      `json:"id"`
	Version         int         `json:"version"`
	ContentStatus   string      `json:"content_status"`
	Modality        Modality    `json:"modality"`
	MuscleGroup     MuscleGroup `json:"muscle_group"`
	Intensity       Intensity   `json:"intensity"`
	DurationMinutes int         `json:"duration_minutes"`

	SuitablePhases []CyclePhase `json:"suitable_phases"`
	OptimizingFor  []string     `json:"optimizing_for"`

	RecoveryDemand  string `json:"recovery_demand"`
	TimeCommitment  string `json:"time_commitment"`
	ProgressionRole string `json:"progression_role"`

	Content    SessionContent        `json:"content"`
	PhaseNotes map[CyclePhase]string `json:"phase_notes"`

	Author    *string `json:"author"`
	CreatedAt string  `json:"created_at"`
}

// SessionContent contiene la estructura del workout.
type SessionContent struct {
	Warmup        ContentBlock     `json:"warmup"`
	MainExercises []ExerciseDetail `json:"main_exercises"`
	Cooldown      ContentBlock     `json:"cooldown"`
}

// ContentBlock representa warmup o cooldown.
type ContentBlock struct {
	DurationMinutes int      `json:"duration_minutes"`
	Description     string   `json:"description"`
	Movements       []string `json:"movements"`
}

// ExerciseDetail representa un ejercicio individual con toda su información.
type ExerciseDetail struct {
	Name         string `json:"name"`
	Sets         int    `json:"sets"`
	Reps         string `json:"reps"`
	RestSeconds  int    `json:"rest_seconds"`
	LoadGuidance string `json:"load_guidance"`
	FormCue      string `json:"form_cue"`
}

// MatchesSession devuelve true si esta LibrarySession matchea los criterios
// de búsqueda de una TrainingSession ligera (modality + muscle_group + intensity).
func (ls LibrarySession) MatchesSession(s TrainingSession) bool {
	return ls.Modality == s.Modality &&
		ls.MuscleGroup == s.MuscleGroup &&
		ls.Intensity == s.Intensity
}

// SuitableForPhase devuelve true si esta sesión es apropiada para la fase dada.
func (ls LibrarySession) SuitableForPhase(phase CyclePhase) bool {
	for _, p := range ls.SuitablePhases {
		if p == phase {
			return true
		}
	}
	return false
}

// ============================================================================
// ENRICHED TRAINING PLAN — the final output of the generation pipeline
// ============================================================================
// This is what the user receives. It combines:
//   - The validated weekly structure (from Layer 1 + engine)
//   - Phase-specific annotations (from Layer 2 transformations)
//   - Full session content (from Layer 3 library selector)

// TrainingPlanDay represents a single day in the final plan,
// enriched with annotations and full content.
type TrainingPlanDay struct {
	Weekday      string           `json:"weekday"`
	IsRestDay    bool             `json:"is_rest_day"`
	Session      *TrainingSession `json:"session,omitempty"`
	Content      *LibrarySession  `json:"content,omitempty"`
	PhaseNote    string           `json:"phase_note,omitempty"`
	LoadLabel    string           `json:"load_label,omitempty"`
	JointWarning bool             `json:"joint_warning,omitempty"`
}

// TrainingWeekPlan is the complete weekly plan ready for the frontend.
type TrainingWeekPlan struct {
	Phase           CyclePhase         `json:"phase"`
	Days            [7]TrainingPlanDay `json:"days"`
	RestWeekOffered bool               `json:"rest_week_offered"`
	VivIntroNote    string             `json:"viv_intro_note,omitempty"`
}

// ============================================================================
// TRAINING BUDGET — output of Layer 0, input to Layer 1
// ============================================================================
// Lives in domain because it's a pure data type shared across packages
// (training, usecase, adapters).

// ModalityBudget describes how many sessions of a specific modality
// the plan should include, with optional intensity override.
type ModalityBudget struct {
	Modality          Modality   `json:"modality"`
	Count             int        `json:"count"`
	IntensityOverride *Intensity `json:"intensity_override,omitempty"`
	VivRecommended    bool       `json:"viv_recommended,omitempty"`
}

// WeekBudget is the complete output of the budget calculator.
// It tells Layer 1 (LLM) exactly what to distribute across 7 days.
type WeekBudget struct {
	TotalSessions   int              `json:"total_sessions"`
	Modalities      []ModalityBudget `json:"modalities"`
	DurationMinutes int              `json:"duration_minutes"`
	RestWeekOffered bool             `json:"rest_week_offered"`
}
