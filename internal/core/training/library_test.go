package training_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/training"
)

// ============================================================================
// HELPERS
// ============================================================================

func writeTestSession(t *testing.T, dir string, s domain.LibrarySession) {
	t.Helper()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal session %s: %v", s.ID, err)
	}
	if err := os.WriteFile(filepath.Join(dir, s.ID+".json"), data, 0644); err != nil {
		t.Fatalf("write session %s: %v", s.ID, err)
	}
}

func buildTestLibrary(t *testing.T, sessions []domain.LibrarySession) *training.Library {
	t.Helper()
	dir := t.TempDir()
	for _, s := range sessions {
		writeTestSession(t, dir, s)
	}
	lib, err := training.LoadLibrary(dir)
	if err != nil {
		t.Fatalf("load test library: %v", err)
	}
	return lib
}

func testSession(id string, mod domain.Modality, mg domain.MuscleGroup, intensity domain.Intensity, phases []domain.CyclePhase, recoveryDemand, timeCommitment, progressionRole string) domain.LibrarySession {
	return domain.LibrarySession{
		ID:              id,
		Version:         1,
		ContentStatus:   "draft_llm_generated",
		Modality:        mod,
		MuscleGroup:     mg,
		Intensity:       intensity,
		DurationMinutes: 45,
		SuitablePhases:  phases,
		OptimizingFor:   []string{"maintenance"},
		RecoveryDemand:  recoveryDemand,
		TimeCommitment:  timeCommitment,
		ProgressionRole: progressionRole,
		Content: domain.SessionContent{
			Warmup:        domain.ContentBlock{DurationMinutes: 5, Description: "test", Movements: []string{"test"}},
			MainExercises: []domain.ExerciseDetail{{Name: "test", Sets: 3, Reps: "10", RestSeconds: 60}},
			Cooldown:      domain.ContentBlock{DurationMinutes: 5, Description: "test", Movements: []string{"test"}},
		},
		PhaseNotes: map[domain.CyclePhase]string{
			domain.PhaseMenstrual:   "test",
			domain.PhaseFollicular:  "test",
			domain.PhaseOvulatory:   "test",
			domain.PhaseEarlyLuteal: "test",
			domain.PhaseLateLuteal:  "test",
		},
		CreatedAt: "2026-04-15",
	}
}

var allPhases = []domain.CyclePhase{
	domain.PhaseMenstrual, domain.PhaseFollicular, domain.PhaseOvulatory,
	domain.PhaseEarlyLuteal, domain.PhaseLateLuteal,
}

// ============================================================================
// LOADER TESTS
// ============================================================================

func TestLoadLibrary_ErrorOnEmptyDir(t *testing.T) {
	_, err := training.LoadLibrary(t.TempDir())
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestLoadLibrary_ErrorOnInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{invalid"), 0644)

	_, err := training.LoadLibrary(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadLibrary_ErrorOnMissingID(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "noid.json"), []byte(`{"version":1,"modality":"STRENGTH"}`), 0644)

	_, err := training.LoadLibrary(dir)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestLoadLibrary_LoadsValidSessions(t *testing.T) {
	s := testSession("test_1", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate, allPhases, "moderate", "high", "build")
	lib := buildTestLibrary(t, []domain.LibrarySession{s})

	if lib.SessionCount() != 1 {
		t.Errorf("expected 1 session, got %d", lib.SessionCount())
	}
}

func TestLoadLibrary_LoadsFromSubdirectories(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "strength")
	os.MkdirAll(sub, 0755)

	s := testSession("sub_1", domain.ModalityStrength, domain.MuscleGroupUpper, domain.IntensityHigh, allPhases, "high", "high", "build")
	data, _ := json.Marshal(s)
	os.WriteFile(filepath.Join(sub, "sub_1.json"), data, 0644)

	lib, err := training.LoadLibrary(dir)
	if err != nil {
		t.Fatalf("failed to load from subdirectory: %v", err)
	}
	if lib.SessionCount() != 1 {
		t.Errorf("expected 1 session from subdirectory, got %d", lib.SessionCount())
	}
}

// ============================================================================
// SELECTOR TESTS — HARD FILTERS
// ============================================================================

func TestSelectSession_MatchesByModalityAndMuscleGroup(t *testing.T) {
	lib := buildTestLibrary(t, []domain.LibrarySession{
		testSession("s_upper", domain.ModalityStrength, domain.MuscleGroupUpper, domain.IntensityModerate, allPhases, "moderate", "high", "maintain"),
		testSession("s_lower", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate, allPhases, "moderate", "high", "maintain"),
	})

	result, found := lib.SelectSession(training.SelectionCriteria{
		Session: domain.TrainingSession{
			Modality:    domain.ModalityStrength,
			MuscleGroup: domain.MuscleGroupLower,
			Intensity:   domain.IntensityModerate,
		},
		Phase: domain.PhaseFollicular,
	})

	if !found {
		t.Fatal("expected to find a session")
	}
	if result.ID != "s_lower" {
		t.Errorf("expected s_lower, got %s", result.ID)
	}
}

func TestSelectSession_RespectsPhaseFilter(t *testing.T) {
	// Session only suitable for follicular
	lib := buildTestLibrary(t, []domain.LibrarySession{
		testSession("follicular_only", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityHigh,
			[]domain.CyclePhase{domain.PhaseFollicular}, "high", "high", "build"),
	})

	// Search in late luteal → should not find it
	_, found := lib.SelectSession(training.SelectionCriteria{
		Session: domain.TrainingSession{
			Modality:    domain.ModalityStrength,
			MuscleGroup: domain.MuscleGroupLower,
			Intensity:   domain.IntensityHigh,
		},
		Phase: domain.PhaseLateLuteal,
	})

	if found {
		t.Error("should not find session unsuitable for this phase")
	}
}

func TestSelectSession_NotFound(t *testing.T) {
	lib := buildTestLibrary(t, []domain.LibrarySession{
		testSession("s1", domain.ModalityStrength, domain.MuscleGroupUpper, domain.IntensityModerate, allPhases, "moderate", "high", "maintain"),
	})

	// Search for HIIT — no HIIT sessions in library
	_, found := lib.SelectSession(training.SelectionCriteria{
		Session: domain.TrainingSession{
			Modality:    domain.ModalityHIIT,
			MuscleGroup: domain.MuscleGroupFullBody,
			Intensity:   domain.IntensityHigh,
		},
		Phase: domain.PhaseFollicular,
	})

	if found {
		t.Error("should not find HIIT session when only STRENGTH exists")
	}
}

// ============================================================================
// SELECTOR TESTS — SOFT FILTERS (CHECK-IN DIMENSIONS)
// ============================================================================

func TestSelectSession_ExcludesHighRecoveryWhenCapacityLow(t *testing.T) {
	lib := buildTestLibrary(t, []domain.LibrarySession{
		testSession("heavy", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate, allPhases, "high", "high", "build"),
		testSession("light", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate, allPhases, "moderate", "high", "maintain"),
	})

	result, found := lib.SelectSession(training.SelectionCriteria{
		Session: domain.TrainingSession{
			Modality:    domain.ModalityStrength,
			MuscleGroup: domain.MuscleGroupLower,
			Intensity:   domain.IntensityModerate,
		},
		Phase:            domain.PhaseFollicular,
		RecoveryCapacity: "low",
	})

	if !found {
		t.Fatal("expected to find a session")
	}
	if result.ID != "light" {
		t.Errorf("expected light session when recovery is low, got %s", result.ID)
	}
}

func TestSelectSession_FallsBackWhenAllHighRecovery(t *testing.T) {
	// Only high recovery sessions available — should still return something
	lib := buildTestLibrary(t, []domain.LibrarySession{
		testSession("heavy_only", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate, allPhases, "high", "high", "build"),
	})

	_, found := lib.SelectSession(training.SelectionCriteria{
		Session: domain.TrainingSession{
			Modality:    domain.ModalityStrength,
			MuscleGroup: domain.MuscleGroupLower,
			Intensity:   domain.IntensityModerate,
		},
		Phase:            domain.PhaseFollicular,
		RecoveryCapacity: "low",
	})

	if !found {
		t.Fatal("should fall back to high recovery session when nothing else available")
	}
}

func TestSelectSession_PrefersLowTimeCommitmentWhenBandwidthLow(t *testing.T) {
	lib := buildTestLibrary(t, []domain.LibrarySession{
		testSession("gym", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate, allPhases, "moderate", "high", "maintain"),
		testSession("home", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate, allPhases, "moderate", "low", "maintain"),
	})

	// Run multiple times — "home" should be selected most of the time
	homeCount := 0
	for i := 0; i < 20; i++ {
		result, found := lib.SelectSession(training.SelectionCriteria{
			Session: domain.TrainingSession{
				Modality:    domain.ModalityStrength,
				MuscleGroup: domain.MuscleGroupLower,
				Intensity:   domain.IntensityModerate,
			},
			Phase:         domain.PhaseFollicular,
			LifeBandwidth: "low",
		})
		if found && result.ID == "home" {
			homeCount++
		}
	}

	if homeCount < 10 {
		t.Errorf("expected 'home' to be preferred when bandwidth is low, got it %d/20 times", homeCount)
	}
}

func TestSelectSession_PrefersBuildWhenReadyToPush(t *testing.T) {
	lib := buildTestLibrary(t, []domain.LibrarySession{
		testSession("recover", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate, allPhases, "moderate", "high", "recover"),
		testSession("build", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate, allPhases, "moderate", "high", "build"),
	})

	buildCount := 0
	for i := 0; i < 20; i++ {
		result, found := lib.SelectSession(training.SelectionCriteria{
			Session: domain.TrainingSession{
				Modality:    domain.ModalityStrength,
				MuscleGroup: domain.MuscleGroupLower,
				Intensity:   domain.IntensityModerate,
			},
			Phase:          domain.PhaseFollicular,
			BuildReadiness: "push_forward",
		})
		if found && result.ID == "build" {
			buildCount++
		}
	}

	if buildCount < 10 {
		t.Errorf("expected 'build' to be preferred when pushing forward, got it %d/20 times", buildCount)
	}
}

func TestSelectSession_PrefersRecoverWhenPullingBack(t *testing.T) {
	lib := buildTestLibrary(t, []domain.LibrarySession{
		testSession("build", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate, allPhases, "moderate", "high", "build"),
		testSession("recover", domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate, allPhases, "moderate", "high", "recover"),
	})

	recoverCount := 0
	for i := 0; i < 20; i++ {
		result, found := lib.SelectSession(training.SelectionCriteria{
			Session: domain.TrainingSession{
				Modality:    domain.ModalityStrength,
				MuscleGroup: domain.MuscleGroupLower,
				Intensity:   domain.IntensityModerate,
			},
			Phase:          domain.PhaseFollicular,
			BuildReadiness: "pull_back",
		})
		if found && result.ID == "recover" {
			recoverCount++
		}
	}

	if recoverCount < 10 {
		t.Errorf("expected 'recover' to be preferred when pulling back, got it %d/20 times", recoverCount)
	}
}

// ============================================================================
// INTEGRATION TEST — carga la librería real
// ============================================================================

func TestLoadLibrary_RealLibrary(t *testing.T) {
	dir := testLibraryDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("library dir not found at %s — skipping", dir)
	}

	lib, err := training.LoadLibrary(dir)
	if err != nil {
		t.Fatalf("failed to load real library: %v", err)
	}

	t.Logf("real library loaded: %d sessions", lib.SessionCount())

	// Verificar que podemos seleccionar al menos una sesión de cada modalidad
	modalities := []struct {
		mod domain.Modality
		mg  domain.MuscleGroup
		i   domain.Intensity
	}{
		{domain.ModalityStrength, domain.MuscleGroupLower, domain.IntensityModerate},
		{domain.ModalityStrength, domain.MuscleGroupUpper, domain.IntensityHigh},
		{domain.ModalityHIIT, domain.MuscleGroupFullBody, domain.IntensityHigh},
		{domain.ModalityPilates, domain.MuscleGroupCore, domain.IntensityLow},
		{domain.ModalityCardio, domain.MuscleGroupNone, domain.IntensityLow},
	}

	for _, m := range modalities {
		result, found := lib.SelectSession(training.SelectionCriteria{
			Session: domain.TrainingSession{
				Modality:    m.mod,
				MuscleGroup: m.mg,
				Intensity:   m.i,
			},
			Phase: domain.PhaseFollicular,
		})

		if !found {
			t.Errorf("no session found for %s/%s/%s", m.mod, m.mg, m.i)
		} else {
			t.Logf("  %s/%s/%s → %s", m.mod, m.mg, m.i, result.ID)
		}
	}
}

func testLibraryDir() string {
	// Project root
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "internal", "content", "training")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
