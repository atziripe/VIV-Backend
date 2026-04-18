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
