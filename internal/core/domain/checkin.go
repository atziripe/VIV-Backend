package domain

import "time"

type Checkin struct {
	ID        string
	UserID    string
	WeekStart time.Time
	CreatedAt time.Time

	Sleep        CheckinSleep    // "How did you sleep last week?"
	Body         CheckinBody     // "How does your body feel today?"
	AppetiteV2   CheckinAppetite // "How has your appetite been?" (renamed to avoid collision)
	Demand       CheckinDemand   // "How demanding was last week?" (NEW)
	LastWeekFeel CheckinLastWeek // "Last week's plan felt..."

	CycleStart  *time.Time          // period start date if it arrived this week
	PMSSymptoms *CheckinPMSSymptoms // PMS Symptoms faced during that week (just in late luteal)

	Predictability CheckinPredictability // "What does your week ahead look like?"
	Readiness      CheckinReadiness      // "Going into next week, you feel..."
	// ModalitySkip holds the modalities the user wants to skip this week.
	// Populated from the "Anything different this week?" check-in question.
	// Empty slice means no preference change ("Same as usual").
	// "lighter" is a special value meaning the user wants reduced intensity.
	ModalitySkip    string `json:"modality_skip,omitempty"`
	AdditionalNotes string
}

// ============================================================================
// CHECK-IN RAW — las 7 respuestas tal como vienen de la UI
// ============================================================================

// CheckinSleep representa la respuesta a "How did you sleep last week?"
type CheckinSleep string

const (
	SleepConsistent   CheckinSleep = "Consistent"
	SleepInconsistent CheckinSleep = "Inconsistent"
	SleepFragmented   CheckinSleep = "Fragmented/Short"
	SleepPoor         CheckinSleep = "Poor + unrested"
)

// CheckinBody representa la respuesta a "How does your body feel today?"
type CheckinBody string

const (
	BodyStrong    CheckinBody = "Strong & responsive"
	BodyNormal    CheckinBody = "Normal"
	BodyHeavier   CheckinBody = "Heavier than usual"
	BodySensitive CheckinBody = "Sensitive or reactive"
)

// CheckinAppetite representa la respuesta a "How has your appetite been?"
// Se guarda pero NO se usa en el algoritmo de planning para MVP.
type CheckinAppetite string

const (
	AppetiteLower   CheckinAppetite = "Lower than usual"
	AppetiteStable  CheckinAppetite = "Stable"
	AppetiteHigher  CheckinAppetite = "Higher"
	AppetiteAllOver CheckinAppetite = "All over the place"
)

// CheckinDemand - "How demanding does life feel?"
type CheckinDemand string

const (
	DemandLight      CheckinDemand = "Light, I had margin"
	DemandManageable CheckinDemand = "Manageable"
	DemandHigh       CheckinDemand = "High, a lot happening"
	DemandOverloaded CheckinDemand = "Overloaded"
)

// CheckinPredictability - "What does your week ahead look like?"
type CheckinPredictability string

const (
	PredictabilityPredictable     CheckinPredictability = "Predictable — I know my schedule"
	PredictabilitySemiPredictable CheckinPredictability = "Semi-predictable — some moving parts"
	PredictabilityUnpredictable   CheckinPredictability = "Unpredictable — every day is different"
	PredictabilityChaotic         CheckinPredictability = "Chaotic — no control"
)

// CheckinLastWeek - "Last week's plan felt..."
type CheckinLastWeek string

const (
	LastWeekTooMuch  CheckinLastWeek = "Too much"
	LastWeekBalanced CheckinLastWeek = "Well balanced"
	LastWeekTooLight CheckinLastWeek = "Too light"
)

// CheckinReadiness - "Going into next week, you feel..."
type CheckinReadiness string

const (
	ReadinessBuild     CheckinReadiness = "Ready to build"
	ReadinessFragile   CheckinReadiness = "Okay but fragile"
	ReadinessStabilize CheckinReadiness = "In need of stabilization"
)

// PMSSymptoms - "Do you experience any of these PMS synptoms"
type CheckinPMSSymptoms string

const (
	PMSCramps     CheckinPMSSymptoms = "Cramps"
	PMSEnergyDips CheckinPMSSymptoms = "Energy dips"
	PMSAnxiety    CheckinPMSSymptoms = "Anxiety"
	PMSCravings   CheckinPMSSymptoms = "Cravings"
	PMSBloating   CheckinPMSSymptoms = "Bloating"
	PMSHeadache   CheckinPMSSymptoms = "Headache"
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
