package strengthlibrary

import "testing"

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

func TestLoadBlockConfig_EveryApprovedMovementGroupIsCoveredExactlyOnce(t *testing.T) {
	lib, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	cfg, err := LoadBlockConfig()
	if err != nil {
		t.Fatalf("LoadBlockConfig(): %v", err)
	}

	for _, zone := range AllZones {
		seenIn := map[MovementGroup]string{}
		for _, slot := range cfg.SlotsForZone(zone) {
			for _, g := range slot.MovementGroups {
				if other, dup := seenIn[g]; dup {
					t.Errorf("zone %q: movement group %q claimed by both %q and %q", zone, g, other, slot.Name)
				}
				seenIn[g] = slot.Name
			}
		}
		for g := range lib.ApprovedExercises(zone) {
			if _, ok := seenIn[g]; !ok {
				t.Errorf("zone %q: movement group %q has approved exercises but no slot covers it", zone, g)
			}
		}
	}
}

func testBlockConfigJSON() []byte {
	return []byte(`{
		"zones": {
			"lower": {
				"slots": [
					{"name": "Squat", "priority": "Main", "movement_groups": ["Squat"]},
					{"name": "Hip Thrust", "priority": "Accessory", "movement_groups": ["Hip thrust / bridge"]},
					{"name": "Calves", "priority": "Optional", "movement_groups": ["Calves"]}
				]
			}
		},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1}
	}`)
}

func TestLoadBlockConfigFromJSON_RejectsUnknownZone(t *testing.T) {
	bad := []byte(`{"zones":{"middle":{"slots":[{"name":"X","priority":"Main","movement_groups":["Y"]}]}},"rotation_policy":{"Main":"hold","Accessory":1,"Optional":1}}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for an unknown zone, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsUnknownPriority(t *testing.T) {
	bad := []byte(`{"zones":{"lower":{"slots":[{"name":"X","priority":"Secondary","movement_groups":["Y"]}]}},"rotation_policy":{"Main":"hold","Accessory":1,"Optional":1}}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for an unknown priority, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsMissingRotationPolicy(t *testing.T) {
	bad := []byte(`{"zones":{"lower":{"slots":[{"name":"X","priority":"Main","movement_groups":["Y"]}]}},"rotation_policy":{"Main":"hold"}}`)
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
			"lower": {
				"slots": [
					{"name": "Calves", "priority": "Optional", "movement_groups": ["Calves"], "rotation_period_blocks": 2}
				]
			}
		},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1}
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
