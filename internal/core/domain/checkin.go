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
	Sleep        CheckinSleep    // "How did you sleep last week?"
	Body         CheckinBody     // "How does your body feel today?"
	AppetiteV2   CheckinAppetite // "How has your appetite been?" (renamed to avoid collision)
	Demand       CheckinDemand   // "How demanding was last week?" (NEW)
	LastWeekFeel CheckinLastWeek // "Last week's plan felt..."

	CycleStart  *time.Time          // period start date if it arrived this week
	PMSSymptoms *CheckinPMSSymptoms // PMS Symptoms faced during that week (just in late luteal)

	Predictability  CheckinPredictability // "What does your week ahead look like?"
	Readiness       CheckinReadiness      // "Going into next week, you feel..."
	AdditionalNotes string
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

// CheckinDemand - "How demanding does life feel?"
type CheckinDemand string

const (
	DemandLight      CheckinDemand = "light"
	DemandManageable CheckinDemand = "manageable"
	DemandHigh       CheckinDemand = "high"
	DemandOverloaded CheckinDemand = "overloaded"
)

// CheckinPredictability - "What does your week ahead look like?"
type CheckinPredictability string

const (
	PredictabilityPredictable     CheckinPredictability = "predictable"
	PredictabilitySemiPredictable CheckinPredictability = "semi_predictable"
	PredictabilityUnpredictable   CheckinPredictability = "unpredictable"
	PredictabilityChaotic         CheckinPredictability = "chaotic"
)

// CheckinLastWeek - "Last week's plan felt..."
type CheckinLastWeek string

const (
	LastWeekTooMuch  CheckinLastWeek = "too_much"
	LastWeekBalanced CheckinLastWeek = "well_balanced"
	LastWeekTooLight CheckinLastWeek = "too_light"
)

// CheckinReadiness - "Going into next week, you feel..."
type CheckinReadiness string

const (
	ReadinessBuild     CheckinReadiness = "ready_to_build"
	ReadinessFragile   CheckinReadiness = "okay_but_fragile"
	ReadinessStabilize CheckinReadiness = "in_need_of_stabilization"
)

// PMSSymptoms - "Do you experience any of these PMS synptoms"
type CheckinPMSSymptoms string

const (
	PMSCramps     CheckinPMSSymptoms = "cramps"
	PMSEnergyDips CheckinPMSSymptoms = "energy_dips"
	PMSAnxiety    CheckinPMSSymptoms = "anxiety"
	PMSCravings   CheckinPMSSymptoms = "cravings"
	PMSBloating   CheckinPMSSymptoms = "bloating"
	PMSHeadache   CheckinPMSSymptoms = "headache"
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
