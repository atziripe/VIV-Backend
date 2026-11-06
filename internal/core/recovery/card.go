package recovery

import (
	"viv/internal/core/activity"
	"viv/internal/core/domain"
)

// ============================================================================
// RECOVERY CARD — DERIVATION LOGIC
// ============================================================================
// Implements the "VIV Recovery Engine — Decision Table & Protocol Spec":
// session-based, diagnostic recovery cost/protocol, replacing the old
// cycle-phase-driven banner pipeline (removed).
//
// Every judgment call the spec itself didn't pin down is marked inline
// below with the same convention internal/core/activity/taxonomy.go uses
// for its own first-draft data, since this logic is built directly on top
// of that data (Type.TrainingEffect, Type.RecoveryCost):
//
//	first-draft, pending product/clinical sign-off
// ============================================================================

// EffectCategory buckets activity.TrainingEffect's six canonical values
// into the five categories the decision table spec's protocol matrix
// reasons about (§2). A session whose TrainingEffect set spans more than
// one category collapses to EffectMixedHybrid — e.g. HIIT's concurrent
// [AnaerobicCapacity, AerobicBase] — matching the spec's own "Mixed/
// Hybrid (circuits, metcon-style)" row.
//
// first-draft, pending product/clinical sign-off: the spec doesn't give
// this mapping explicitly, only the two five/six-item lists.
type EffectCategory string

const (
	EffectAerobic        EffectCategory = "aerobic"
	EffectAnaerobic      EffectCategory = "anaerobic"
	EffectStrengthNeural EffectCategory = "strength_neural"
	EffectMixedHybrid    EffectCategory = "mixed_hybrid"
	EffectMobility       EffectCategory = "mobility"
)

func categoryFor(effect activity.TrainingEffect) EffectCategory {
	switch effect {
	case activity.EffectStrength, activity.EffectHypertrophy:
		return EffectStrengthNeural
	case activity.EffectAnaerobicCapacity:
		return EffectAnaerobic
	case activity.EffectAerobicBase:
		return EffectAerobic
	case activity.EffectMobility, activity.EffectRecovery:
		return EffectMobility
	default:
		return EffectMobility
	}
}

// CategorizeSession derives a session's effect category from its
// activity+intensity, reading activity.Type.TrainingEffect directly — ok
// is false for an activity/intensity combination the taxonomy doesn't
// define (should not happen for a real generated day, but never guessed
// at rather than silently defaulting).
func CategorizeSession(activityID activity.ID, intensity activity.IntensityLevel) (cat EffectCategory, ok bool) {
	t, found := activity.ByID(activityID)
	if !found {
		return "", false
	}
	effects, found := t.TrainingEffect[intensity]
	if !found || len(effects) == 0 {
		return "", false
	}

	seen := map[EffectCategory]bool{}
	for _, e := range effects {
		seen[categoryFor(e)] = true
	}
	if len(seen) > 1 {
		return EffectMixedHybrid, true
	}
	for c := range seen {
		return c, true
	}
	return "", false
}

// SessionInputs is what DeriveCostTier needs about the session recovery
// is being evaluated FROM — decision table spec §1.
type SessionInputs struct {
	ActivityType    activity.ID
	Intensity       activity.IntensityLevel
	MuscleGroup     activity.MuscleGroup
	DurationMinutes int
}

// longSessionMinutes is the Short/Long duration-modifier threshold (§1) —
// the spec doesn't give a cutoff.
//
// first-draft, pending product/clinical sign-off.
const longSessionMinutes = 45

// DeriveCostTier computes the base Low/Medium/High recovery cost for a
// session (spec §1-2): activity.Type.RecoveryCost (already a first-draft
// 0-3 literature-reviewed value per activity+intensity, see taxonomy.go)
// is the starting score, bumped by one tier if the session was full-body
// OR long — the two content modifiers don't stack past a single combined
// bump, so a full-body AND long session isn't pushed two tiers at once.
// The universal modifier layer (§3 — sleep debt etc.) applies separately,
// after this — see BumpTierForSleepDebt.
//
// first-draft, pending product/clinical sign-off: the spec says cost is
// "derived from the combination" of these inputs but doesn't give the
// exact scoring rule — this is that rule, not a value from the spec
// itself.
func DeriveCostTier(in SessionInputs) (tier domain.RecoveryCostTier, ok bool) {
	t, found := activity.ByID(in.ActivityType)
	if !found {
		return "", false
	}
	base, found := t.RecoveryCost[in.Intensity]
	if !found {
		return "", false
	}

	bump := 0
	if in.MuscleGroup == activity.MuscleGroupFullBody {
		bump = 1
	}
	if in.DurationMinutes >= longSessionMinutes {
		bump = 1
	}

	score := base + bump
	if score > 3 {
		score = 3
	}

	switch {
	case score <= 1:
		return domain.RecoveryCostLow, true
	case score == 2:
		return domain.RecoveryCostMedium, true
	default:
		return domain.RecoveryCostHigh, true
	}
}

// BumpTierForSleepDebt applies the sleep-debt universal modifier (spec
// §3): poor sleep the prior night bumps recovery cost up one tier
// regardless of session type, capped at High. "Sleep is the precondition
// for the other recovery mechanisms to function" — the spec's own
// reasoning for why this overrides rather than just adds a tip.
func BumpTierForSleepDebt(tier domain.RecoveryCostTier) domain.RecoveryCostTier {
	if tier == domain.RecoveryCostLow {
		return domain.RecoveryCostMedium
	}
	return domain.RecoveryCostHigh
}
