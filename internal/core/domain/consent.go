package domain

import "time"

// ConsentPurpose identifies which processing activity a consent record
// covers. Deliberately a small, closed set rather than a free-text field —
// each purpose maps to a specific, real gate in the code (see the usecases
// that check it), not an aspirational list. health_data_processing is not
// "optional" in practice (the product doesn't function without it); it's
// still captured explicitly rather than bundled into generic terms-of-use
// acceptance, because GDPR Art. 9 requires that for special-category data.
type ConsentPurpose string

const (
	// ConsentHealthDataProcessing covers onboarding and everything the app
	// does with cycle/training/nutrition data — required to create an
	// account at all. Revoking it is handled as account deletion (see
	// SetConsentUseCase), not as "continue using the app without it."
	ConsentHealthDataProcessing ConsentPurpose = "health_data_processing"

	// ConsentAIProcessing covers only the two OpenAI calls that have a real
	// non-AI fallback today: the weekly note and meal-copy polish (see
	// GetWeeklyNoteUseCase and mealgen's AsyncCopyEnricher/
	// AsyncNutritionPlanCopyEnricher). Weekly *scheduling*'s OpenAI call is
	// deliberately NOT gated by this — there's no fallback for it yet, so
	// gating it on an optional consent would make that opt-out illusory.
	// It stays bundled under ConsentHealthDataProcessing/contract necessity
	// until a real non-AI scheduling path exists.
	ConsentAIProcessing ConsentPurpose = "ai_processing"

	// ConsentResearchProductImprovement is unused by any pipeline today —
	// captured now, ahead of any actual use, so that using it later for
	// aggregate/research purposes doesn't breach purpose limitation
	// (GDPR Art. 5.1.b: a purpose not disclosed at collection time can't be
	// added after the fact without fresh consent).
	ConsentResearchProductImprovement ConsentPurpose = "research_product_improvement"
)

// CurrentPrivacyPolicyVersion is stamped onto every new Consent record.
// Bump it whenever the privacy policy's purposes change — SetConsentUseCase
// has no logic that depends on the exact string, but a client can compare
// a user's stored PolicyVersion against this to know their consent is
// stale and needs re-asking.
const CurrentPrivacyPolicyVersion = "1.0"

// Consent is one grant-or-revoke event — append-only by design (see
// ConsentRepository.Grant): every change is a new record, never an
// overwrite, so there's always a full history to point to if asked to
// demonstrate consent was given (GDPR Art. 7.1). The *current* state of a
// purpose is simply its most recent record.
type Consent struct {
	UserID        string
	Purpose       ConsentPurpose
	Granted       bool
	PolicyVersion string
	CreatedAt     time.Time
}
