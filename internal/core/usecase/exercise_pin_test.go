package usecase_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/checkin"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
	"viv/internal/core/mesocycle"
	"viv/internal/core/strengthlibrary"
	"viv/internal/core/usecase"
	weeklytarget "viv/internal/core/weekly_target"
)

// fakeExercisePinRepo is an in-memory ExercisePinRepository, tracking how
// many times Save is called (per key) so tests can prove reuse never
// triggers a write.
type fakeExercisePinRepo struct {
	pins      map[string]*mesocycle.PinnedExerciseSet // "userID/activityType/muscleGroup" -> pin
	saveCalls int
}

func newFakeExercisePinRepo() *fakeExercisePinRepo {
	return &fakeExercisePinRepo{pins: map[string]*mesocycle.PinnedExerciseSet{}}
}

func pinKey(userID string, activityType activity.ID, muscleGroup activity.MuscleGroup) string {
	return userID + "/" + string(activityType) + "/" + string(muscleGroup)
}

func (f *fakeExercisePinRepo) Get(_ context.Context, userID string, activityType activity.ID, muscleGroup activity.MuscleGroup) (*mesocycle.PinnedExerciseSet, error) {
	p, ok := f.pins[pinKey(userID, activityType, muscleGroup)]
	if !ok {
		return nil, nil
	}
	cp := *p
	return &cp, nil
}

func (f *fakeExercisePinRepo) Save(_ context.Context, userID string, pin mesocycle.PinnedExerciseSet) error {
	f.saveCalls++
	cp := pin
	f.pins[pinKey(userID, pin.ActivityType, pin.MuscleGroup)] = &cp
	return nil
}

// testFixtureLibrary/testFixtureConfig give every muscle group at least
// one slot with 2+ approved exercises, so these integration-level tests
// don't need to care about thin-slot/rotation specifics — those are
// mesocycle's own tests' job. Here we only care about create/reuse/rotate
// and that a cascade adjustment never re-triggers selection.
func testFixtureLibrary(t *testing.T) *strengthlibrary.Library {
	t.Helper()
	lib, err := strengthlibrary.NewLibraryFromJSON([]byte(`[
		{"id":"lower_a","activity_type":"strength","name":"Lower A","zone":"lower","movement_group":"Squat","review_status":"approved"},
		{"id":"lower_b","activity_type":"strength","name":"Lower B","zone":"lower","movement_group":"Squat","review_status":"approved"},
		{"id":"upper_a","activity_type":"strength","name":"Upper A","zone":"upper","movement_group":"Horizontal push","review_status":"approved"},
		{"id":"upper_b","activity_type":"strength","name":"Upper B","zone":"upper","movement_group":"Horizontal push","review_status":"approved"},
		{"id":"core_a","activity_type":"strength","name":"Core A","zone":"core","movement_group":"Anti-extension","review_status":"approved"},
		{"id":"core_b","activity_type":"strength","name":"Core B","zone":"core","movement_group":"Anti-extension","review_status":"approved"}
	]`))
	if err != nil {
		t.Fatalf("testFixtureLibrary: %v", err)
	}
	return lib
}

func testFixtureConfig(t *testing.T) *strengthlibrary.BlockConfig {
	t.Helper()
	cfg, err := strengthlibrary.NewBlockConfigFromJSON([]byte(`{
		"zones": {
			"lower": {"slots": [{"name": "Squat", "priority": "Main", "movement_groups": ["Squat"]}]},
			"upper": {"slots": [{"name": "Push", "priority": "Accessory", "movement_groups": ["Horizontal push"]}]},
			"full":  {"slots": [{"name": "Squat", "priority": "Main", "movement_groups": ["Squat"]}]},
			"core":  {"slots": [{"name": "Anti-Extension", "priority": "Main", "movement_groups": ["Anti-extension"]}]}
		},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1}
	}`))
	if err != nil {
		t.Fatalf("testFixtureConfig: %v", err)
	}
	return cfg
}

func newTestPinService(t *testing.T, repo *fakeExercisePinRepo, today time.Time) *usecase.ExercisePinService {
	return usecase.NewExercisePinServiceWithClock(
		repo, mesocycle.DefaultBlockExerciseSelector{}, testFixtureLibrary(t), testFixtureConfig(t),
		func() time.Time { return today },
	)
}

// ============================================================================
// First session creates and persists a pinned set
// ============================================================================

func TestExercisePinService_FirstSessionCreatesAndPersistsPin(t *testing.T) {
	repo := newFakeExercisePinRepo()
	today := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	svc := newTestPinService(t, repo, today)

	ids, err := svc.ResolvePinnedExercises(context.Background(), "u1", activity.Strength, activity.MuscleGroupLower)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("expected a non-empty exercise set")
	}
	if repo.saveCalls != 1 {
		t.Errorf("saveCalls = %d, want 1", repo.saveCalls)
	}

	stored, err := repo.Get(context.Background(), "u1", activity.Strength, activity.MuscleGroupLower)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if stored == nil {
		t.Fatal("expected a persisted pin")
	}
	if stored.BlockNumber != 1 {
		t.Errorf("BlockNumber = %d, want 1", stored.BlockNumber)
	}
	if len(stored.ActiveExercises) != 1 || stored.ActiveExercises["Squat"] != "lower_a" {
		t.Errorf("ActiveExercises = %v, want {Squat: lower_a} (block 1 = pool[0])", stored.ActiveExercises)
	}
	if !stored.ActivatedOn.Equal(today) {
		t.Errorf("ActivatedOn = %v, want %v", stored.ActivatedOn, today)
	}
	if !stored.RotatesOn.Equal(today.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays)) {
		t.Errorf("RotatesOn = %v, want %d days after ActivatedOn", stored.RotatesOn, mesocycle.StrengthMesocycleLengthDays)
	}
}

// ============================================================================
// Subsequent sessions within 28 days reuse the exact same set
// ============================================================================

func TestExercisePinService_SubsequentSessionsReuseSameSet(t *testing.T) {
	repo := newFakeExercisePinRepo()
	day0 := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	first, err := newTestPinService(t, repo, day0).ResolvePinnedExercises(context.Background(), "u1", activity.Strength, activity.MuscleGroupUpper)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Three more "sessions" on different days, all still inside the cycle.
	for _, day := range []time.Time{day0.AddDate(0, 0, 2), day0.AddDate(0, 0, 10), day0.AddDate(0, 0, 27)} {
		got, err := newTestPinService(t, repo, day).ResolvePinnedExercises(context.Background(), "u1", activity.Strength, activity.MuscleGroupUpper)
		if err != nil {
			t.Fatalf("day=%v: unexpected error: %v", day, err)
		}
		if !reflect.DeepEqual(got, first) {
			t.Errorf("day=%v: ExerciseIDs = %v, want the exact same set as day 0: %v", day, got, first)
		}
	}

	if repo.saveCalls != 1 {
		t.Errorf("saveCalls = %d, want 1 (only the very first call should have written anything)", repo.saveCalls)
	}
}

// ============================================================================
// Day 29+ triggers rotation to a new set
// ============================================================================

func TestExercisePinService_RotatesOnDay29(t *testing.T) {
	repo := newFakeExercisePinRepo()
	day0 := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	svc0 := newTestPinService(t, repo, day0)
	if _, err := svc0.ResolvePinnedExercises(context.Background(), "u1", activity.Strength, activity.MuscleGroupCore); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	before, err := repo.Get(context.Background(), "u1", activity.Strength, activity.MuscleGroupCore)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	day29 := day0.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays)
	svc29 := newTestPinService(t, repo, day29)
	if _, err := svc29.ResolvePinnedExercises(context.Background(), "u1", activity.Strength, activity.MuscleGroupCore); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after, err := repo.Get(context.Background(), "u1", activity.Strength, activity.MuscleGroupCore)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	if after.ActivatedOn.Equal(before.ActivatedOn) {
		t.Error("expected ActivatedOn to have moved to day 29")
	}
	if !after.ActivatedOn.Equal(day29) {
		t.Errorf("ActivatedOn = %v, want %v", after.ActivatedOn, day29)
	}
	if !after.RotatesOn.Equal(day29.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays)) {
		t.Errorf("new RotatesOn = %v, want 28 days after day 29", after.RotatesOn)
	}
	if after.BlockNumber != before.BlockNumber+1 {
		t.Errorf("BlockNumber = %d, want %d (before+1)", after.BlockNumber, before.BlockNumber+1)
	}
	// Core's "Anti-Extension" slot is Main (holds) in the fixture config,
	// per-group members in this test's library are both approved, but a
	// Main slot stays at pool[0] regardless — same exercise before/after.
	if after.ActiveExercises["Anti-extension"] != before.ActiveExercises["Anti-extension"] {
		t.Errorf("Main slot exercise changed on rotation: before=%v after=%v", before.ActiveExercises, after.ActiveExercises)
	}
	// Rotation is a real write, distinct from a same-cycle reuse: creation
	// (day 0) + rotation (day 29) = 2 saves.
	if repo.saveCalls != 2 {
		t.Errorf("saveCalls = %d, want 2 (creation + rotation)", repo.saveCalls)
	}
}

// ============================================================================
// Non-eligible activity types
// ============================================================================

func TestExercisePinService_NonEligibleActivityReturnsNilWithoutTouchingRepo(t *testing.T) {
	repo := newFakeExercisePinRepo()
	svc := newTestPinService(t, repo, time.Now())

	ids, err := svc.ResolvePinnedExercises(context.Background(), "u1", activity.HIIT, activity.MuscleGroupFullBody)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids != nil {
		t.Errorf("expected nil exercise IDs for a non-eligible activity, got %v", ids)
	}
	if repo.saveCalls != 0 {
		t.Errorf("expected no repository writes for a non-eligible activity, saveCalls = %d", repo.saveCalls)
	}
}

// ============================================================================
// The cascade never changes which exercises are pinned
// ============================================================================

// TestExercisePinService_CascadeAdjustmentNeverChangesPinnedExercises is
// the required proof: a real cascade.ResolveSlotConflict run (reusing the
// exact ProtectType+RelaxImpact scenario from VIV-107's own tests) changes
// Intensity/Impact/LoadTier but leaves ActivityType and MuscleGroup
// exactly as they were. Since ExercisePinService's lookup key is only
// (activityType, muscleGroup), resolving the pin before and after the
// cascade adjustment must return the identical set with exactly one
// underlying write — proving the adjustment never re-triggers selection.
func TestExercisePinService_CascadeAdjustmentNeverChangesPinnedExercises(t *testing.T) {
	repo := newFakeExercisePinRepo()
	today := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	svc := newTestPinService(t, repo, today)

	original := cascade.Activity{
		ActivityType: activity.Strength,
		Intensity:    activity.IntensityH,
		Impact:       activity.ImpactM,
		MuscleGroup:  activity.MuscleGroupLower,
	}

	// Resolve the pin BEFORE any adjustment, exactly as weekly generation
	// (VIV-106) would when first assigning this slot.
	before, err := svc.ResolvePinnedExercises(context.Background(), "u1", original.ActivityType, original.MuscleGroup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Same bad-readiness scenario as VIV-107's tests: forces ProtectType
	// then RelaxImpact, ending at Strength@(L,L) — same ActivityType/MuscleGroup.
	g, ok := goal.ByID(goal.StrengthMuscle)
	if !ok {
		t.Fatal("goal.ByID(StrengthMuscle) not found")
	}
	target, err := weeklytarget.BuildWeeklyTarget(g, checkin.RecoveryLow, checkin.BandwidthLow, checkin.BuildPullBack, domain.PhaseFollicular)
	if err != nil {
		t.Fatalf("unexpected error building target: %v", err)
	}

	resolved, err := cascade.ResolveSlotConflict(target, original, cascade.UserCatalog{Activities: []activity.ID{activity.Strength}}, g, 0)
	if err != nil {
		t.Fatalf("unexpected error from cascade: %v", err)
	}
	if resolved.Intensity == original.Intensity && resolved.Impact == original.Impact {
		t.Fatal("sanity check failed — expected the cascade to actually change intensity/impact")
	}
	if resolved.ActivityType != original.ActivityType || resolved.MuscleGroup != original.MuscleGroup {
		t.Fatalf("sanity check failed — expected ActivityType/MuscleGroup unchanged by the cascade, got %s/%s",
			resolved.ActivityType, resolved.MuscleGroup)
	}

	// Resolve the pin again using the cascade's OUTPUT identity — this is
	// what daily adaptation (VIV-107) would do for the same slot.
	after, err := svc.ResolvePinnedExercises(context.Background(), "u1", resolved.ActivityType, resolved.MuscleGroup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(before, after) {
		t.Errorf("pinned exercises changed after a cascade intensity/impact adjustment: before=%v after=%v", before, after)
	}
	if repo.saveCalls != 1 {
		t.Errorf("saveCalls = %d, want 1 — the cascade adjustment must not have triggered re-selection", repo.saveCalls)
	}
}
