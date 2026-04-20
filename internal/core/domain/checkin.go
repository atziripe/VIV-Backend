package domain

import "time"

type Checkin struct {
	ID        string
	UserID    string
	WeekStart time.Time
	CreatedAt time.Time

	// ============================================================
	// V2 fields — new check-in questions (used by training planner)
	// ============================================================
	Sleep          CheckinSleep          // "How did you sleep last week?"
	Body           CheckinBody           // "How does your body feel today?"
	AppetiteV2     CheckinAppetite       // "How has your appetite been?" (renamed to avoid collision)
	Demand         CheckinDemand         // "How demanding does life feel?" (NEW)
	Predictability CheckinPredictability // "What does your week ahead look like?"
	LastWeekFeel   CheckinLastWeek       // "Last week's plan felt..."
	Readiness      CheckinReadiness      // "Going into next week, you feel..."

	CycleStart *time.Time // period start date if it arrived this week

	// ============================================================
	// V1 fields — deprecated, kept for backward compatibility
	// These were used when the check-in went directly to the LLM.
	// New code should use V2 fields above.
	// Remove once V1 data is no longer needed.
	// ============================================================
	SleepQuality       string // → replaced by Sleep
	BodyStatus         string // → replaced by Body
	Appetite           string // → replaced by AppetiteV2
	StressLevel        string // → no direct replacement (was LLM-interpreted)
	LastWeekFeeling    string // → replaced by LastWeekFeel
	WorkloadPrediction string // → replaced by Predictability + Demand
	MentalEnergy       string // → replaced by Readiness
	PromptVersion      string // meta — keep for audit
}

// ============================================================================
// CHECK-IN RAW — las 7 respuestas tal como vienen de la UI
// ============================================================================

// CheckinSleep representa la respuesta a "How did you sleep last week?"
type CheckinSleep string

const (
	SleepConsistent   CheckinSleep = "consistent"
	SleepInconsistent CheckinSleep = "inconsistent"
	SleepFragmented   CheckinSleep = "fragmented"
	SleepPoor         CheckinSleep = "poor"
)

// CheckinBody representa la respuesta a "How does your body feel today?"
type CheckinBody string

const (
	BodyEnergised CheckinBody = "energised"
	BodyNeutral   CheckinBody = "neutral"
	BodySensitive CheckinBody = "sensitive"
	BodyInflamed  CheckinBody = "inflamed"
)

// CheckinAppetite representa la respuesta a "How has your appetite been?"
// Se guarda pero NO se usa en el algoritmo de planning para MVP.
type CheckinAppetite string

const (
	AppetiteLower   CheckinAppetite = "lower"
	AppetiteStable  CheckinAppetite = "stable"
	AppetiteHigher  CheckinAppetite = "higher"
	AppetiteAllOver CheckinAppetite = "all_over"
)

// CheckinDemand representa la respuesta a "How demanding does life feel?"
type CheckinDemand string

const (
	DemandLight      CheckinDemand = "light"
	DemandManageable CheckinDemand = "manageable"
	DemandHigh       CheckinDemand = "high"
	DemandOverloaded CheckinDemand = "overloaded"
)

// CheckinPredictability representa la respuesta a "What does your week ahead look like?"
type CheckinPredictability string

const (
	PredictabilityPredictable     CheckinPredictability = "predictable"
	PredictabilitySemiPredictable CheckinPredictability = "semi_predictable"
	PredictabilityUnpredictable   CheckinPredictability = "unpredictable"
	PredictabilityChaotic         CheckinPredictability = "chaotic"
)

// CheckinLastWeek representa la respuesta a "Last week's plan felt..."
type CheckinLastWeek string

const (
	LastWeekTooMuch  CheckinLastWeek = "too_much"
	LastWeekBalanced CheckinLastWeek = "well_balanced"
	LastWeekTooLight CheckinLastWeek = "too_light"
)

// CheckinReadiness representa la respuesta a "Going into next week, you feel..."
type CheckinReadiness string

const (
	ReadinessBuild     CheckinReadiness = "ready_to_build"
	ReadinessFragile   CheckinReadiness = "okay_but_fragile"
	ReadinessStabilize CheckinReadiness = "in_need_of_stabilization"
)

// ============================================================================
// DIMENSIONES DERIVADAS — lo que el planner realmente consume
// ============================================================================

// RecoveryCapacity indica qué tan recuperada está la usuaria.
type RecoveryCapacity string

const (
	RecoveryLow      RecoveryCapacity = "low"
	RecoveryModerate RecoveryCapacity = "moderate"
	RecoveryHigh     RecoveryCapacity = "high"
)

// LifeBandwidth indica cuánto ancho de banda tiene para entrenar.
type LifeBandwidth string

const (
	BandwidthLow      LifeBandwidth = "low"
	BandwidthModerate LifeBandwidth = "moderate"
	BandwidthHigh     LifeBandwidth = "high"
)

// BuildReadiness indica la dirección del ajuste de carga.
type BuildReadiness string

const (
	ReadinessPullBack    BuildReadiness = "pull_back"
	ReadinessMaintain    BuildReadiness = "maintain"
	ReadinessPushForward BuildReadiness = "push_forward"
)

// CheckinDimensions agrupa las 3 dimensiones derivadas del check-in.
// Es lo que el budget calculator, el selector de librería, y el trigger
// de rest week consumen.
type CheckinDimensions struct {
	Recovery  RecoveryCapacity
	Bandwidth LifeBandwidth
	Build     BuildReadiness
}

// RestWeekRecommended devuelve true si se debería ofrecer rest week.
// Trigger: al menos 2 de 3 dimensiones en su estado más bajo.
func (d CheckinDimensions) RestWeekRecommended() bool {
	lowCount := 0
	if d.Recovery == RecoveryLow {
		lowCount++
	}
	if d.Bandwidth == BandwidthLow {
		lowCount++
	}
	if d.Build == ReadinessPullBack {
		lowCount++
	}
	return lowCount >= 2
}
