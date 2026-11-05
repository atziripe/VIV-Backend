package strengthlibrary

import "testing"

// minimalPrescriptionJSON is the smallest valid status/exercise_types/
// weekly_prescription_by_type/variant_rules section a BlockConfig
// fixture needs to pass validation — these tests don't exercise
// prescription lookups at all, so the exercise name/type code are
// arbitrary placeholders.
const minimalPrescriptionJSON = `
	"status": "test_fixture",
	"exercise_types": {"Placeholder Exercise": {"type": "C", "per_side": false}},
	"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
	"variant_rules": {
		"short": {"drop_order": ["Optional", "Accessory"], "never_drop": ["Main"], "min_exercises": 3},
		"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
		"simplified": {"active": false},
		"intensity_mapping": {
			"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2},
			"Medium": "as_written",
			"High": "treated_as_Medium_until_reviewed"
		}
	},
	"warmup_cooldown": {
		"_status": "test_fixture",
		"applies_to_all_tiers": true,
		"ramp_up_note": "Before the first Main exercise, do 1-2 light sets.",
		"zones": {
			"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["Leg swings"], "cooldown": ["Quad stretch"]}
		}
	},
	"rest_seconds_by_type": {
		"_status": "test_fixture",
		"C": [90, 90, 120, 120]
	}
`

func TestLoadBlockConfig_EmbeddedConfigIsValid(t *testing.T) {
	cfg, err := LoadBlockConfig()
	if err != nil {
		t.Fatalf("LoadBlockConfig(): %v", err)
	}
	for _, zone := range AllZones {
		if len(cfg.SlotsForZone(zone)) == 0 {
			t.Errorf("zone %q has no slots in the embedded config", zone)
		}
	}
}

// TestLoadBlockConfig_NoSlotReferencesAnUncoveredMovementGroup checks
// the direction that IS a real invariant: every movement group a slot
// lists must actually have approved exercises in the real catalog
// (catching a typo'd group name). The REVERSE direction — "every
// approved group must be covered by some slot" — is deliberately NOT
// checked: the real config intentionally leaves some approved groups
// (e.g. Full body's "Core — anti-extension"/"...anti-rotation"/
// "...anti-lateral flexion", Core zone's "Trunk flexion"/"...pelvic
// curl") out of the structured block rotation entirely. That's the
// config author's call, not a gap for this package to flag.
func TestLoadBlockConfig_NoSlotReferencesAnUncoveredMovementGroup(t *testing.T) {
	lib, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	cfg, err := LoadBlockConfig()
	if err != nil {
		t.Fatalf("LoadBlockConfig(): %v", err)
	}

	for _, zone := range AllZones {
		approved := lib.ApprovedExercises(zone)
		for _, slot := range cfg.SlotsForZone(zone) {
			for _, g := range slot.MovementGroups {
				if len(approved[g]) == 0 {
					t.Errorf("zone %q slot %q references movement group %q, which has no approved exercises", zone, slot.Name, g)
				}
			}
		}
	}
}

func testBlockConfigJSON() []byte {
	return []byte(`{
		"zones": {
			"Lower body": [
				{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"], "pool_order": ["Placeholder Exercise"]},
				{"slot": "Hip Thrust", "priority": "Accessory", "source_groups": ["Hip thrust / bridge"], "pool_order": ["Placeholder Exercise"]},
				{"slot": "Calves", "priority": "Optional", "source_groups": ["Calves"], "pool_order": ["Placeholder Exercise"]}
			]
		},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},` + minimalPrescriptionJSON + `
	}`)
}

func TestLoadBlockConfigFromJSON_RejectsUnknownZone(t *testing.T) {
	bad := []byte(`{"zones":{"Middle body":[{"slot":"X","priority":"Main","source_groups":["Y"]}]},"rotation_policy":{"Main":"hold","Accessory":1,"Optional":1}}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for an unknown zone, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsUnknownPriority(t *testing.T) {
	bad := []byte(`{"zones":{"Lower body":[{"slot":"X","priority":"Secondary","source_groups":["Y"]}]},"rotation_policy":{"Main":"hold","Accessory":1,"Optional":1}}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for an unknown priority, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsMissingRotationPolicy(t *testing.T) {
	bad := []byte(`{"zones":{"Lower body":[{"slot":"X","priority":"Main","source_groups":["Y"]}]},"rotation_policy":{"Main":"hold"}}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for a rotation_policy missing a priority, got nil")
	}
}

func TestBlockConfig_HoldsForeverAndRotationPeriod(t *testing.T) {
	cfg, err := NewBlockConfigFromJSON(testBlockConfigJSON())
	if err != nil {
		t.Fatalf("NewBlockConfigFromJSON: %v", err)
	}
	slots := cfg.SlotsForZone(ZoneLower)

	byName := map[string]Slot{}
	for _, s := range slots {
		byName[s.Name] = s
	}

	if !cfg.HoldsForever(byName["Squat"]) {
		t.Error("Main slot should hold forever")
	}
	if cfg.HoldsForever(byName["Hip Thrust"]) {
		t.Error("Accessory slot should not hold forever")
	}
	if got := cfg.RotationPeriodBlocks(byName["Hip Thrust"]); got != 1 {
		t.Errorf("Accessory default rotation period = %d, want 1", got)
	}
}

func TestBlockConfig_SlotOverrideChangesRotationPeriod(t *testing.T) {
	withOverride := []byte(`{
		"zones": {
			"Lower body": [
				{"slot": "Calves", "priority": "Optional", "source_groups": ["Calves"], "pool_order": ["Placeholder Exercise"], "rotation_period_blocks": 2}
			]
		},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},` + minimalPrescriptionJSON + `
	}`)
	cfg, err := NewBlockConfigFromJSON(withOverride)
	if err != nil {
		t.Fatalf("NewBlockConfigFromJSON: %v", err)
	}
	slot := cfg.SlotsForZone(ZoneLower)[0]
	if cfg.HoldsForever(slot) {
		t.Error("an explicit rotation_period_blocks override must never hold forever")
	}
	if got := cfg.RotationPeriodBlocks(slot); got != 2 {
		t.Errorf("overridden rotation period = %d, want 2 (the config's own value, not the Optional default of 1)", got)
	}
}
