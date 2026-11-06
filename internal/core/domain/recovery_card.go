package domain

import "time"

// ============================================================================
// RECOVERY CARD — the compact, session-driven recovery view (VIV Recovery
// Engine Decision Table spec). Replaces the old paragraph-banner recovery
// response entirely — GET /recovery/today and its domain.RecoveryDay were
// removed. See internal/core/recovery/card.go for the derivation logic
// that builds one of these.
// ============================================================================

// RecoveryCostTier is the derived Low/Medium/High recovery cost — distinct
// from activity.Type.RecoveryCost's raw 0-3 scale, which this tier is
// computed from (see recovery.DeriveCostTier). "" means the tier doesn't
// apply (a training day has nothing to recover FROM yet).
type RecoveryCostTier string

const (
	RecoveryCostLow    RecoveryCostTier = "low"
	RecoveryCostMedium RecoveryCostTier = "medium"
	RecoveryCostHigh   RecoveryCostTier = "high"
)

// RecoveryActionItem is one recommended action (or, at High cost, a
// restriction) shown on the card — the one Primary item shown up front,
// or one of up to two Secondary items collapsed behind "see more."
type RecoveryActionItem struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// RecoveryActionKind identifies which button the user tapped — see
// SaveRecoveryActionUseCase. Done/NotToday pair with a Low/Medium-cost
// card's recommended action; Acknowledged/TrainedAnyway pair with a
// High-cost card's restriction (design doc mockup's "Got it" / "I
// trained"). A training day's card has no action buttons at all — the
// client's "See today's meals" affordance is pure navigation, not
// something this domain needs to know about.
type RecoveryActionKind string

const (
	RecoveryActionDone          RecoveryActionKind = "done"
	RecoveryActionNotToday      RecoveryActionKind = "not_today"
	RecoveryActionAcknowledged  RecoveryActionKind = "acknowledged"
	RecoveryActionTrainedAnyway RecoveryActionKind = "trained_anyway"
)

// RecoveryAction is the persisted record of what the user actually did
// with a given day's card — one per (UserID, Date). Distinct from
// domain.SessionLog: this logs a response to a recovery recommendation,
// not a training session.
type RecoveryAction struct {
	UserID   string
	Date     time.Time
	Kind     RecoveryActionKind
	LoggedAt time.Time
}

// RecoveryCard is the full response for GET /recovery/card.
type RecoveryCard struct {
	Date      string `json:"date"`
	Weekday   string `json:"weekday"`
	IsRestDay bool   `json:"is_rest_day"`

	// CostTier is "" on a training day — see RecoveryCostTier's doc.
	CostTier RecoveryCostTier `json:"cost_tier,omitempty"`

	Headline string `json:"headline"`
	Context  string `json:"context"`

	Primary   *RecoveryActionItem  `json:"primary,omitempty"`
	Secondary []RecoveryActionItem `json:"secondary,omitempty"`

	// Why is one short sentence explaining the cost tier — only meant to
	// be shown inside the client's expanded "see more" state, alongside
	// Secondary.
	Why string `json:"why,omitempty"`

	// AvailableActions lists exactly which buttons the client should
	// render for this card, in order — empty on a training day (no
	// recovery action to log there).
	AvailableActions []RecoveryActionOption `json:"available_actions,omitempty"`

	// LoggedAction echoes back whatever the user already logged for this
	// date (see SaveRecoveryActionUseCase) — nil if nothing logged yet,
	// so the client knows whether to show the action buttons or the
	// already-acted state.
	LoggedAction *RecoveryActionKind `json:"logged_action,omitempty"`

	// RescheduleNote is set only when this card's generation actually
	// pushed a too-soon high-intensity session to a later day (see
	// GetRecoveryCardUseCase) — "" means nothing was moved.
	RescheduleNote string `json:"reschedule_note,omitempty"`
}

// RecoveryActionOption is one action button the client should render.
type RecoveryActionOption struct {
	Kind  RecoveryActionKind `json:"kind"`
	Label string             `json:"label"`
}
