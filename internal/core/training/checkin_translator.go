package training

import "viv/internal/core/domain"

// TranslateCheckin convierte las 7 respuestas crudas del check-in semanal
// en las 3 dimensiones derivadas que el planner consume.
//
// Algoritmo: sistema de puntos por dimensión. Cada respuesta suma o resta
// puntos. El total se mapea a low/moderate/high (o pull_back/maintain/push_forward).
//
// Los pesos son conservadores para MVP — se iteran con data real de uso.
// If the checkin is empty (new user, no check-in yet), returns healthy defaults.
func TranslateCheckin(checkin domain.Checkin) domain.CheckinDimensions {
	if isEmptyCheckin(checkin) {
		return defaultDimensions()
	}

	return domain.CheckinDimensions{
		Recovery:  computeRecovery(checkin),
		Bandwidth: computeBandwidth(checkin),
		Build:     computeBuild(checkin),
	}
}

// ============================================================================
// DIMENSION 1 — RecoveryCapacity
// ============================================================================
// Inputs: Sleep + Body (Appetite stored but excluded from algorithm)
// Score range: -2 to +4
// Mapping: ≤0 → low, 1-2 → moderate, 3-4 → high

func computeRecovery(c domain.Checkin) domain.RecoveryCapacity {
	score := sleepScore(c.Sleep) + bodyScore(c.Body)

	switch {
	case score >= 3:
		return domain.RecoveryHigh
	case score >= 1:
		return domain.RecoveryModerate
	default:
		return domain.RecoveryLow
	}
}

func sleepScore(s domain.CheckinSleep) int {
	switch s {
	case domain.SleepConsistent:
		return 2
	case domain.SleepInconsistent:
		return 1
	case domain.SleepFragmented:
		return 0
	case domain.SleepPoor:
		return -1
	default:
		return 0
	}
}

func bodyScore(b domain.CheckinBody) int {
	switch b {
	case domain.BodyEnergised:
		return 2
	case domain.BodyNeutral:
		return 1
	case domain.BodySensitive:
		return 0
	case domain.BodyInflamed:
		return -1
	default:
		return 0
	}
}

// ============================================================================
// DIMENSION 2 — LifeBandwidth
// ============================================================================
// Inputs: Demand + Predictability
// Score range: -2 to +4
// Mapping: ≤0 → low, 1-2 → moderate, 3-4 → high

func computeBandwidth(c domain.Checkin) domain.LifeBandwidth {
	score := demandScore(c.Demand) + predictabilityScore(c.Predictability)

	switch {
	case score >= 3:
		return domain.BandwidthHigh
	case score >= 1:
		return domain.BandwidthModerate
	default:
		return domain.BandwidthLow
	}
}

func demandScore(d domain.CheckinDemand) int {
	switch d {
	case domain.DemandLight:
		return 2
	case domain.DemandManageable:
		return 1
	case domain.DemandHigh:
		return 0
	case domain.DemandOverloaded:
		return -1
	default:
		return 0
	}
}

func predictabilityScore(p domain.CheckinPredictability) int {
	switch p {
	case domain.PredictabilityPredictable:
		return 2
	case domain.PredictabilitySemiPredictable:
		return 1
	case domain.PredictabilityUnpredictable:
		return 0
	case domain.PredictabilityChaotic:
		return -1
	default:
		return 0
	}
}

// ============================================================================
// DIMENSION 3 — BuildReadiness
// ============================================================================
// Inputs: LastWeekFeel + Readiness
// Score range: -3 to +2
// Mapping: ≤-1 → pull_back, 0 → maintain, ≥1 → push_forward

func computeBuild(c domain.Checkin) domain.BuildReadiness {
	score := lastWeekScore(c.LastWeekFeel) + readinessScore(c.Readiness)

	switch {
	case score >= 1:
		return domain.ReadinessPushForward
	case score == 0:
		return domain.ReadinessMaintain
	default:
		return domain.ReadinessPullBack
	}
}

func lastWeekScore(l domain.CheckinLastWeek) int {
	switch l {
	case domain.LastWeekTooMuch:
		return -1
	case domain.LastWeekBalanced:
		return 0
	case domain.LastWeekTooLight:
		return 1
	default:
		return 0
	}
}

func readinessScore(r domain.CheckinReadiness) int {
	switch r {
	case domain.ReadinessBuild:
		return 1
	case domain.ReadinessFragile:
		return -1
	case domain.ReadinessStabilize:
		return -2
	default:
		return 0
	}
}

// isEmptyCheckin returns true if the checkin has no meaningful data.
// This happens for new users who haven't done their first weekly check-in.
func isEmptyCheckin(c domain.Checkin) bool {
	return c.Sleep == "" && c.Body == "" && c.Demand == "" && c.Readiness == ""
}

// defaultDimensions returns optimistic defaults for new users.
// The assumption: a new user is starting fresh — give her a normal plan,
// not a reduced one. She'll calibrate via her first check-in.
func defaultDimensions() domain.CheckinDimensions {
	return domain.CheckinDimensions{
		Recovery:  domain.RecoveryModerate,
		Bandwidth: domain.BandwidthModerate,
		Build:     domain.ReadinessMaintain,
	}
}
