package domain

// IneligibilityReason identifies why someone was turned away at the
// pre-onboarding eligibility screen ("Which describes you right now?") —
// VIV's guidance is calibrated to a natural menstrual cycle, so anyone who
// answers anything other than "Natural menstrual cycle" is blocked before
// onboarding starts and their account is deleted immediately (see
// ReportIneligibilityUseCase), not retained under any grace period.
//
// A small, closed set, same reasoning as ConsentPurpose: each value maps to
// one option on that screen, not a free-text field.
type IneligibilityReason string

const (
	IneligibilityPregnantOrPostpartum     IneligibilityReason = "pregnant_or_postpartum"
	IneligibilityPerimenopauseOrMenopause IneligibilityReason = "perimenopause_or_menopause"
	IneligibilityHormonalContraception    IneligibilityReason = "hormonal_contraception"
	IneligibilityNotSure                  IneligibilityReason = "not_sure"
)
