package recovery

import "viv/internal/core/domain"

// ============================================================================
// RECOVERY CARD — PROTOCOL COPY
// ============================================================================
// One CardCopy per (EffectCategory, RecoveryCostTier) cell of the decision
// table spec's protocol matrix (§2), adapted from the spec's own bullet
// text into the compact "one primary action + up to two secondary + one
// why sentence" shape the redesigned card uses — never inventing new
// clinical claims beyond what the spec's protocol bullets already say.
//
// first-draft content, pending product/clinical sign-off — same status as
// every other value this package derives from (activity.Type.RecoveryCost/
// TrainingEffect, both explicitly first-draft in taxonomy.go).
// ============================================================================

type CardCopy struct {
	Headline  string
	Primary   domain.RecoveryActionItem
	Secondary []domain.RecoveryActionItem
	Why       string
}

var mobilityCopy = CardCopy{
	Headline: "Nothing to recover from",
	Primary: domain.RecoveryActionItem{
		Title:  "Nothing needed today",
		Detail: "This session was already the recovery.",
	},
	Why: "Mobility work barely registers as a load — it's built to help you recover, not something to recover from.",
}

var cardCopyTable = map[EffectCategory]map[domain.RecoveryCostTier]CardCopy{
	EffectAerobic: {
		domain.RecoveryCostLow: {
			Headline: "One thing today",
			Primary: domain.RecoveryActionItem{
				Title:  "Hydrate and eat normally",
				Detail: "Nothing special needed — steady-state work like this doesn't leave lasting fatigue.",
			},
			Why: "Steady aerobic work at this length causes very little muscle damage, so there's not much to actively manage.",
		},
		domain.RecoveryCostMedium: {
			Headline: "One thing today",
			Primary: domain.RecoveryActionItem{
				Title:  "Refuel with carbs and electrolytes",
				Detail: "A longer session like this depletes glycogen — replace it within a couple hours.",
			},
			Secondary: []domain.RecoveryActionItem{
				{Title: "Add a light protein source", Detail: "Supports the same recovery window."},
			},
			Why: "A longer steady-state session pulls more from your glycogen stores, even without heavy muscle damage.",
		},
		domain.RecoveryCostHigh: {
			Headline: "Today is for recovering",
			Primary: domain.RecoveryActionItem{
				Title:  "Prioritize rehydration and sodium",
				Detail: "This fatigue is thermal and metabolic, not muscular — water alone won't be enough.",
			},
			Secondary: []domain.RecoveryActionItem{
				{Title: "Move bedtime earlier tonight", Detail: "The main lever here is sleep, not stretching."},
			},
			Why: "Long or hot aerobic sessions cost you through heat and fluid loss more than muscle damage — recovery follows a different path.",
		},
	},
	EffectAnaerobic: {
		domain.RecoveryCostLow: {
			Headline: "One thing today",
			Primary: domain.RecoveryActionItem{
				Title:  "Protein and water, nothing else",
				Detail: "Normal training is fine again tomorrow.",
			},
			Why: "A short, low-volume session at this length doesn't build up much fatigue.",
		},
		domain.RecoveryCostMedium: {
			Headline: "One thing today",
			Primary: domain.RecoveryActionItem{
				Title:  "Protein within a few hours",
				Detail: "A quality source with leucine — this is what rebuilds after glycolytic work.",
			},
			Secondary: []domain.RecoveryActionItem{
				{Title: "Protect tonight's sleep", Detail: "10-20 min of slow breathing or NSDR helps it land."},
				{Title: "Move, don't rest, tomorrow", Detail: "Light movement clears fatigue faster than sitting still."},
			},
			Why: "This kind of session taps your glycolytic system hard enough to need real refueling and a good night's sleep.",
		},
		domain.RecoveryCostHigh: {
			Headline: "Today is for recovering",
			Primary: domain.RecoveryActionItem{
				Title:  "Active recovery tomorrow, not full rest",
				Detail: "Easy movement, not a couch day.",
			},
			Secondary: []domain.RecoveryActionItem{
				{Title: "Magnesium and omega-3", Detail: "Helps with the soreness this level of work leaves behind."},
				{Title: "Check in before your next hard session", Detail: "How sore or ready you feel matters more than the calendar here."},
			},
			Why: "This was a big glycolytic hit — worth respecting before you stack another hard session on it.",
		},
	},
	EffectStrengthNeural: {
		domain.RecoveryCostLow: {
			Headline: "One thing today",
			Primary: domain.RecoveryActionItem{
				Title:  "Normal protein and sleep",
				Detail: "Nothing extra needed for a session this contained.",
			},
			Why: "One muscle group at a moderate load doesn't create much systemic fatigue.",
		},
		domain.RecoveryCostMedium: {
			Headline: "One thing today",
			Primary: domain.RecoveryActionItem{
				Title:  "Protect tonight's sleep",
				Detail: "Your nervous system recovers through sleep, not stretching, after heavy lifting.",
			},
			Secondary: []domain.RecoveryActionItem{
				{Title: "Eat enough protein", Detail: "Supports the same repair window."},
			},
			Why: "Heavy, low-rep work fatigues your nervous system more than your muscles — sleep is what actually resets it.",
		},
		domain.RecoveryCostHigh: {
			Headline: "Today is for recovering",
			Primary: domain.RecoveryActionItem{
				Title:  "Nothing harder than a walk",
				Detail: "No stair sprints, no \"light\" session. Tomorrow needs you.",
			},
			Secondary: []domain.RecoveryActionItem{
				{Title: "Skip ice or NSAIDs right after", Detail: "They can blunt the adaptation you just trained for."},
				{Title: "Plan an extra easy day before your next heavy session", Detail: "CNS fatigue outlasts how sore you feel."},
			},
			Why: "Max-effort lifting hits your nervous system hard enough that soreness fading doesn't mean you're actually recovered.",
		},
	},
	EffectMixedHybrid: {
		domain.RecoveryCostLow: {
			Headline: "One thing today",
			Primary: domain.RecoveryActionItem{
				Title:  "Protein and water",
				Detail: "Treat this like a light anaerobic day.",
			},
			Why: "A short, low-impact circuit like this stays light on both systems.",
		},
		domain.RecoveryCostMedium: {
			Headline: "One thing today",
			Primary: domain.RecoveryActionItem{
				Title:  "Refuel with protein and carbs",
				Detail: "This combines glycogen depletion with muscle damage — both need addressing.",
			},
			Secondary: []domain.RecoveryActionItem{
				{Title: "Protect tonight's sleep", Detail: "10-20 min of NSDR or slow breathing helps."},
			},
			Why: "Circuit-style training taxes your energy stores and your muscles at the same time.",
		},
		domain.RecoveryCostHigh: {
			Headline: "Today is for recovering",
			Primary: domain.RecoveryActionItem{
				Title:  "Active recovery tomorrow, not full rest",
				Detail: "Magnesium and omega-3, protein close to your session tonight.",
			},
			Secondary: []domain.RecoveryActionItem{
				{Title: "Expect lower output for a day or two", Detail: "Glycogen resynthesis takes 24-72h regardless of how you refeed."},
			},
			Why: "This is the heaviest combined load in the system — both your energy stores and your muscles took a real hit.",
		},
	},
}

// CopyFor returns the protocol copy for a session's effect category and
// derived cost tier. Mobility ignores tier — its taxonomy RecoveryCost is
// always 0-1 (see taxonomy.go), so it never legitimately reaches Medium/
// High on its own; the same near-zero copy applies regardless (matching
// the spec's single Mobility row, with no Medium/High cells).
func CopyFor(cat EffectCategory, tier domain.RecoveryCostTier) (CardCopy, bool) {
	if cat == EffectMobility {
		return mobilityCopy, true
	}
	byTier, ok := cardCopyTable[cat]
	if !ok {
		return CardCopy{}, false
	}
	c, ok := byTier[tier]
	return c, ok
}

// TrainingDayCopy is the forward-looking "BUILDING" state shown when
// today itself is a training day — no cost tier applies (nothing to
// recover FROM yet), fixed copy rather than derived, same as the old
// pipeline's variantT.
var TrainingDayCopy = CardCopy{
	Headline: "Nothing to recover from",
	Primary: domain.RecoveryActionItem{
		Title:  "Eat properly two hours before",
		Detail: "The only thing today needs from you in advance.",
	},
	Secondary: []domain.RecoveryActionItem{
		{Title: "Protein and carbs close to your session", Detail: "Sets up how well it converts to adaptation."},
	},
	Why: "What you do after this session determines whether it converts — protein, sleep tonight, and a reset tomorrow.",
}
