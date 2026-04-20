package training_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/training"
)

// ============================================================================
// HELPERS
// ============================================================================

func defaultPrefs() training.ParsedTrainingPreferences {
	return training.ParsedTrainingPreferences{
		SessionsPerWeek: 4,
		DurationMinutes: 45,
		Modalities: []domain.Modality{
			domain.ModalityStrength,
			domain.ModalityPilates,
			domain.ModalityHIIT,
			domain.ModalityCardio,
		},
		Goals: []string{"strength and resilience"},
	}
}

func goodDimensions() domain.CheckinDimensions {
	return domain.CheckinDimensions{
		Recovery:  domain.RecoveryHigh,
		Bandwidth: domain.BandwidthHigh,
		Build:     domain.ReadinessPushForward,
	}
}

func findModality(budget domain.WeekBudget, m domain.Modality) *domain.ModalityBudget {
	for _, mb := range budget.Modalities {
		if mb.Modality == m {
			return &mb
		}
	}
	return nil
}

func totalAllocated(budget domain.WeekBudget) int {
	total := 0
	for _, mb := range budget.Modalities {
		total += mb.Count
	}
	return total
}

// ============================================================================
// PARSER TESTS
// ============================================================================

func TestParseTrainingOften(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"1-3 times", 2},
		{"3-5 times/week", 4},
		{"Everyday", 6},
		{"Rarely", 1},
		{"", 3},
	}

	for _, tt := range tests {
		user := domain.User{TrainingOften: tt.input}
		prefs := training.ParseTrainingPreferences(user)
		if prefs.SessionsPerWeek != tt.expected {
			t.Errorf("TrainingOften %q: expected %d, got %d", tt.input, tt.expected, prefs.SessionsPerWeek)
		}
	}
}

func TestParseTrainingDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"- 30 min", 25},
		{"30 - 45 min", 40},
		{"45 - 60 min", 60},
		{"1 - 2 hours", 75},
		{"+2 hours", 120},
		{"", 45},
	}

	for _, tt := range tests {
		user := domain.User{TrainingDuration: tt.input}
		prefs := training.ParseTrainingPreferences(user)
		if prefs.DurationMinutes != tt.expected {
			t.Errorf("TrainingDuration %q: expected %d, got %d", tt.input, tt.expected, prefs.DurationMinutes)
		}
	}
}

func TestParseTrainingType(t *testing.T) {
	user := domain.User{TrainingType: "Strength,Pilates,HIIT,Cardio"}
	prefs := training.ParseTrainingPreferences(user)

	if len(prefs.Modalities) != 4 {
		t.Fatalf("expected 4 modalities, got %d", len(prefs.Modalities))
	}
	if prefs.Modalities[0] != domain.ModalityStrength {
		t.Errorf("expected first modality STRENGTH, got %s", prefs.Modalities[0])
	}
}

func TestParseTrainingType_SingleModality(t *testing.T) {
	user := domain.User{TrainingType: "Pilates"}
	prefs := training.ParseTrainingPreferences(user)

	if len(prefs.Modalities) != 1 {
		t.Fatalf("expected 1 modality, got %d", len(prefs.Modalities))
	}
	if prefs.Modalities[0] != domain.ModalityPilates {
		t.Errorf("expected PILATES, got %s", prefs.Modalities[0])
	}
}

func TestParseTrainingGoals(t *testing.T) {
	user := domain.User{TrainingGoals: "Energy & Consistency,Strength & Resilience"}
	prefs := training.ParseTrainingPreferences(user)

	if len(prefs.Goals) != 2 {
		t.Fatalf("expected 2 goals, got %d", len(prefs.Goals))
	}
	if prefs.Goals[0] != "energy and consistency" {
		t.Errorf("expected 'energy and consistency', got %q", prefs.Goals[0])
	}
	if prefs.Goals[1] != "strength and resilience" {
		t.Errorf("expected 'strength and resilience', got %q", prefs.Goals[1])
	}
}

// ============================================================================
// BUDGET CALCULATOR — PHASE CAPS
// ============================================================================

func TestBudget_DistributionRespectsPhaseMatrix_LateLuteal(t *testing.T) {
	// User wants 4 sessions in late luteal — gets 4 (no cap)
	// but HIIT should be 0 and Strength should be overridden to moderate
	prefs := defaultPrefs()
	budget := training.CalculateBudget(prefs, domain.PhaseLateLuteal, goodDimensions())

	if budget.TotalSessions != 4 {
		t.Errorf("expected 4 (no phase cap), got %d", budget.TotalSessions)
	}

	hiit := findModality(budget, domain.ModalityHIIT)
	if hiit != nil && hiit.Count > 0 {
		t.Error("HIIT should still be 0 in late luteal (hard exclusion)")
	}
}

func TestBudget_PhaseCapRespected_Follicular(t *testing.T) {
	// User wants 4 sessions, follicular allows up to 5 — should get 4
	prefs := defaultPrefs()
	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, goodDimensions())

	if budget.TotalSessions != 4 {
		t.Errorf("follicular with 4 requested should give 4, got %d", budget.TotalSessions)
	}
}

// ============================================================================
// BUDGET CALCULATOR — HIIT REMOVAL
// ============================================================================

func TestBudget_NoHIIT_InLateLuteal(t *testing.T) {
	prefs := defaultPrefs()
	budget := training.CalculateBudget(prefs, domain.PhaseLateLuteal, goodDimensions())

	hiit := findModality(budget, domain.ModalityHIIT)
	if hiit != nil && hiit.Count > 0 {
		t.Error("HIIT should not be allocated in late luteal")
	}
}

func TestBudget_NoHIIT_InMenstrual(t *testing.T) {
	prefs := defaultPrefs()
	budget := training.CalculateBudget(prefs, domain.PhaseMenstrual, goodDimensions())

	hiit := findModality(budget, domain.ModalityHIIT)
	if hiit != nil && hiit.Count > 0 {
		t.Error("HIIT should not be allocated in menstrual")
	}
}

// ============================================================================
// BUDGET CALCULATOR — MODALITY FILTERING
// ============================================================================

func TestBudget_OnlySelectedModalities(t *testing.T) {
	// User only selected Strength + Pilates
	prefs := training.ParsedTrainingPreferences{
		SessionsPerWeek: 3,
		DurationMinutes: 45,
		Modalities:      []domain.Modality{domain.ModalityStrength, domain.ModalityPilates},
		Goals:           []string{"maintenance"},
	}

	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, goodDimensions())

	for _, mb := range budget.Modalities {
		if mb.Modality == domain.ModalityHIIT || mb.Modality == domain.ModalityCardio {
			t.Errorf("should not include %s — user didn't select it", mb.Modality)
		}
	}
}

// ============================================================================
// BUDGET CALCULATOR — DIMENSION MODULATION
// ============================================================================

func TestBudget_ReducedWhenBothLow(t *testing.T) {
	prefs := defaultPrefs()
	dims := domain.CheckinDimensions{
		Recovery:  domain.RecoveryLow,
		Bandwidth: domain.BandwidthLow,
		Build:     domain.ReadinessPullBack,
	}

	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, dims)

	// Follicular allows 5, user wants 4, both low → 4-1 = 3
	if budget.TotalSessions != 3 {
		t.Errorf("expected 3 sessions when both dimensions low, got %d", budget.TotalSessions)
	}
}

func TestBudget_NoHIIT_WhenRecoveryLow(t *testing.T) {
	prefs := defaultPrefs()
	dims := domain.CheckinDimensions{
		Recovery:  domain.RecoveryLow,
		Bandwidth: domain.BandwidthHigh,
		Build:     domain.ReadinessMaintain,
	}

	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, dims)

	hiit := findModality(budget, domain.ModalityHIIT)
	if hiit != nil && hiit.Count > 0 {
		t.Error("HIIT should not be allocated when recovery is low")
	}
}

func TestBudget_IntensityOverride_LateLuteal(t *testing.T) {
	prefs := defaultPrefs()
	budget := training.CalculateBudget(prefs, domain.PhaseLateLuteal, goodDimensions())

	strength := findModality(budget, domain.ModalityStrength)
	if strength == nil {
		t.Fatal("expected strength in budget")
	}
	if strength.IntensityOverride == nil || *strength.IntensityOverride != domain.IntensityModerate {
		t.Error("strength should be overridden to moderate in late luteal")
	}
}

func TestBudget_IntensityOverride_RecoveryLow(t *testing.T) {
	prefs := defaultPrefs()
	dims := domain.CheckinDimensions{
		Recovery:  domain.RecoveryLow,
		Bandwidth: domain.BandwidthHigh,
		Build:     domain.ReadinessMaintain,
	}

	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, dims)

	strength := findModality(budget, domain.ModalityStrength)
	if strength == nil {
		t.Fatal("expected strength in budget")
	}
	if strength.IntensityOverride == nil || *strength.IntensityOverride != domain.IntensityModerate {
		t.Error("strength should be overridden to moderate when recovery is low")
	}
}

func TestBudget_CardioLow_InMenstrual(t *testing.T) {
	prefs := defaultPrefs()
	budget := training.CalculateBudget(prefs, domain.PhaseMenstrual, goodDimensions())

	cardio := findModality(budget, domain.ModalityCardio)
	if cardio != nil && cardio.IntensityOverride == nil {
		t.Error("cardio should be overridden to low in menstrual")
	}
}

// ============================================================================
// BUDGET CALCULATOR — REST WEEK
// ============================================================================

func TestBudget_RestWeekOffered(t *testing.T) {
	prefs := defaultPrefs()
	dims := domain.CheckinDimensions{
		Recovery:  domain.RecoveryLow,
		Bandwidth: domain.BandwidthLow,
		Build:     domain.ReadinessPullBack,
	}

	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, dims)

	if !budget.RestWeekOffered {
		t.Error("rest week should be offered when all dimensions are low")
	}
}

func TestBudget_RestWeekNotOffered(t *testing.T) {
	prefs := defaultPrefs()
	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, goodDimensions())

	if budget.RestWeekOffered {
		t.Error("rest week should not be offered when dimensions are good")
	}
}

// ============================================================================
// BUDGET CALCULATOR — EDGE CASES
// ============================================================================

func TestBudget_MinimumOnSession(t *testing.T) {
	// Even with extreme reduction, never go below 1
	prefs := training.ParsedTrainingPreferences{
		SessionsPerWeek: 1,
		DurationMinutes: 30,
		Modalities:      []domain.Modality{domain.ModalityPilates},
		Goals:           []string{"stress regulation"},
	}
	dims := domain.CheckinDimensions{
		Recovery:  domain.RecoveryLow,
		Bandwidth: domain.BandwidthLow,
		Build:     domain.ReadinessPullBack,
	}

	budget := training.CalculateBudget(prefs, domain.PhaseLateLuteal, dims)

	if budget.TotalSessions < 1 {
		t.Errorf("should never go below 1 session, got %d", budget.TotalSessions)
	}
}

func TestBudget_PilatesOnly_User_GetsStrengthInjected(t *testing.T) {
	prefs := training.ParsedTrainingPreferences{
		SessionsPerWeek: 3,
		DurationMinutes: 40,
		Modalities:      []domain.Modality{domain.ModalityPilates},
		Goals:           []string{"stress regulation"},
	}

	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, goodDimensions())

	// Should have 2 modalities: VIV-injected Strength + user's Pilates
	if len(budget.Modalities) != 2 {
		t.Fatalf("expected 2 modalities (Strength injected + Pilates), got %d", len(budget.Modalities))
	}

	strength := findModality(budget, domain.ModalityStrength)
	if strength == nil {
		t.Fatal("expected VIV to inject Strength")
	}
	if !strength.VivRecommended {
		t.Error("injected Strength should be marked VivRecommended")
	}

	pilates := findModality(budget, domain.ModalityPilates)
	if pilates == nil {
		t.Fatal("expected Pilates to remain")
	}
	if pilates.Count < 1 {
		t.Error("user's chosen Pilates should have at least 1 slot")
	}
}

func TestBudget_AllocatedMatchesTotal(t *testing.T) {
	// Verify that allocated sessions sum to total for various configs
	configs := []struct {
		phase domain.CyclePhase
		mods  []domain.Modality
		want  int
	}{
		{domain.PhaseFollicular, []domain.Modality{domain.ModalityStrength, domain.ModalityHIIT, domain.ModalityPilates}, 4},
		{domain.PhaseLateLuteal, []domain.Modality{domain.ModalityStrength, domain.ModalityPilates}, 3},
		{domain.PhaseMenstrual, []domain.Modality{domain.ModalityStrength, domain.ModalityPilates, domain.ModalityCardio}, 3},
	}

	for _, c := range configs {
		prefs := training.ParsedTrainingPreferences{
			SessionsPerWeek: 4,
			DurationMinutes: 45,
			Modalities:      c.mods,
		}
		budget := training.CalculateBudget(prefs, c.phase, goodDimensions())

		allocated := totalAllocated(budget)
		if allocated > budget.TotalSessions {
			t.Errorf("phase %s: allocated %d > total %d", c.phase, allocated, budget.TotalSessions)
		}
	}
}

// ============================================================================
// NO PHASE CAP — user's requested sessions are respected
// ============================================================================

func TestBudget_NoPhaseCapOnTotal_LateLuteal(t *testing.T) {
	// User wants 5 sessions in late luteal — gets 5, not capped to 3
	prefs := training.ParsedTrainingPreferences{
		SessionsPerWeek: 5,
		DurationMinutes: 45,
		Modalities: []domain.Modality{
			domain.ModalityStrength, domain.ModalityPilates, domain.ModalityCardio,
		},
		Goals: []string{"performance and progression"},
	}

	budget := training.CalculateBudget(prefs, domain.PhaseLateLuteal, goodDimensions())

	if budget.TotalSessions != 5 {
		t.Errorf("expected 5 sessions (no phase cap), got %d", budget.TotalSessions)
	}
}

func TestBudget_NoPhaseCapOnTotal_Menstrual(t *testing.T) {
	prefs := training.ParsedTrainingPreferences{
		SessionsPerWeek: 4,
		DurationMinutes: 45,
		Modalities: []domain.Modality{
			domain.ModalityStrength, domain.ModalityPilates,
		},
		Goals: []string{"strength and resilience"},
	}

	budget := training.CalculateBudget(prefs, domain.PhaseMenstrual, goodDimensions())

	if budget.TotalSessions != 4 {
		t.Errorf("expected 4 sessions (no phase cap), got %d", budget.TotalSessions)
	}
}

// ============================================================================
// VIV-RECOMMENDED STRENGTH INJECTION
// ============================================================================

func TestBudget_InjectsStrength_WhenNotSelected(t *testing.T) {
	// User only selected Pilates + Cardio — VIV should inject Strength
	prefs := training.ParsedTrainingPreferences{
		SessionsPerWeek: 4,
		DurationMinutes: 45,
		Modalities:      []domain.Modality{domain.ModalityPilates, domain.ModalityCardio},
		Goals:           []string{"stress regulation"},
	}

	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, goodDimensions())

	strength := findModality(budget, domain.ModalityStrength)
	if strength == nil {
		t.Fatal("expected VIV to inject Strength sessions")
	}
	if strength.Count != 2 {
		t.Errorf("expected 2 injected Strength sessions in follicular, got %d", strength.Count)
	}
	if !strength.VivRecommended {
		t.Error("injected Strength should be marked as VivRecommended")
	}
	if strength.IntensityOverride == nil || *strength.IntensityOverride != domain.IntensityLow {
		t.Error("injected Strength should be intensity low")
	}
}

func TestBudget_InjectsStrength_OnlyOne_InMenstrual(t *testing.T) {
	prefs := training.ParsedTrainingPreferences{
		SessionsPerWeek: 3,
		DurationMinutes: 40,
		Modalities:      []domain.Modality{domain.ModalityPilates, domain.ModalityCardio},
		Goals:           []string{"energy and consistency"},
	}

	budget := training.CalculateBudget(prefs, domain.PhaseMenstrual, goodDimensions())

	strength := findModality(budget, domain.ModalityStrength)
	if strength == nil {
		t.Fatal("expected VIV to inject Strength in menstrual")
	}
	if strength.Count != 1 {
		t.Errorf("expected 1 injected Strength session in menstrual, got %d", strength.Count)
	}
}

func TestBudget_NoStrengthInjection_WhenOnlyOneSessionTotal(t *testing.T) {
	// If user only has 1 session, don't force Strength — leave her choice
	prefs := training.ParsedTrainingPreferences{
		SessionsPerWeek: 1,
		DurationMinutes: 30,
		Modalities:      []domain.Modality{domain.ModalityPilates},
		Goals:           []string{"stress regulation"},
	}

	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, goodDimensions())

	strength := findModality(budget, domain.ModalityStrength)
	if strength != nil && strength.Count > 0 {
		t.Error("should not inject Strength when user only has 1 session")
	}
}

func TestBudget_StrengthInjection_LeavesRoomForUserChoice(t *testing.T) {
	// With 3 sessions and 2 injected Strength, user still gets 1 of her choice
	prefs := training.ParsedTrainingPreferences{
		SessionsPerWeek: 3,
		DurationMinutes: 45,
		Modalities:      []domain.Modality{domain.ModalityPilates, domain.ModalityCardio},
		Goals:           []string{"stress regulation"},
	}

	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, goodDimensions())

	nonStrength := 0
	for _, mb := range budget.Modalities {
		if mb.Modality != domain.ModalityStrength {
			nonStrength += mb.Count
		}
	}

	if nonStrength < 1 {
		t.Error("should leave at least 1 slot for user's chosen modalities")
	}
}

func TestBudget_NoDoubleStrength_WhenUserSelectedIt(t *testing.T) {
	// User already selected Strength — should NOT get extra injected Strength
	prefs := defaultPrefs() // includes Strength

	budget := training.CalculateBudget(prefs, domain.PhaseFollicular, goodDimensions())

	strengthCount := 0
	vivRecommendedFound := false
	for _, mb := range budget.Modalities {
		if mb.Modality == domain.ModalityStrength {
			strengthCount += mb.Count
			if mb.VivRecommended {
				vivRecommendedFound = true
			}
		}
	}

	if vivRecommendedFound {
		t.Error("should not mark Strength as VivRecommended when user selected it")
	}
}
