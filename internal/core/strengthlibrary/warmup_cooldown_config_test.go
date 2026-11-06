package strengthlibrary

import "testing"

func TestLoadBlockConfig_EmbeddedWarmupCooldownIsValidForEveryZone(t *testing.T) {
	cfg, err := LoadBlockConfig()
	if err != nil {
		t.Fatalf("LoadBlockConfig(): %v", err)
	}
	if cfg.WarmupCooldownStatus() == "" {
		t.Error("expected a non-empty WarmupCooldownStatus")
	}
	if cfg.RampUpNote() == "" {
		t.Error("expected a non-empty RampUpNote")
	}
	for _, zone := range AllZones {
		wc, ok := cfg.WarmupCooldownForZone(zone)
		if !ok {
			t.Errorf("zone %q has no warmup_cooldown entry in the embedded config", zone)
			continue
		}
		if len(wc.Warmup) == 0 {
			t.Errorf("zone %q has an empty warmup list", zone)
		}
		if len(wc.Cooldown) == 0 {
			t.Errorf("zone %q has an empty cooldown list", zone)
		}
		if wc.WarmupMinutes < 1 || wc.CooldownMinutes < 1 {
			t.Errorf("zone %q has a non-positive warmup/cooldown duration: %+v", zone, wc)
		}
	}
}

func TestBlockConfig_WarmupCooldownForZone_UnknownZoneReturnsFalse(t *testing.T) {
	cfg, err := NewBlockConfigFromJSON(testBlockConfigJSON())
	if err != nil {
		t.Fatalf("NewBlockConfigFromJSON: %v", err)
	}
	if _, ok := cfg.WarmupCooldownForZone(ZoneCore); ok {
		t.Error("expected ok=false for a zone with no warmup_cooldown entry in this fixture (only Lower body is defined)")
	}
}

func baseWarmupCooldownConfigJSON(warmupCooldownOverride string) []byte {
	return []byte(`{
		"status": "x",
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Placeholder Exercise": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 1},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		},
		"warmup_cooldown": ` + warmupCooldownOverride + `
	}`)
}

func TestLoadBlockConfigFromJSON_RejectsMissingWarmupCooldown(t *testing.T) {
	bad := []byte(`{
		"status": "x",
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Placeholder Exercise": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 1},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		}
	}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for a config with no warmup_cooldown section at all, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsMissingWarmupCooldownStatus(t *testing.T) {
	bad := baseWarmupCooldownConfigJSON(`{
		"applies_to_all_tiers": true,
		"ramp_up_note": "note",
		"zones": {"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["a"], "cooldown": ["b"]}}
	}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for a missing warmup_cooldown._status, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsAppliesToAllTiersFalse(t *testing.T) {
	bad := baseWarmupCooldownConfigJSON(`{
		"_status": "x",
		"applies_to_all_tiers": false,
		"ramp_up_note": "note",
		"zones": {"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["a"], "cooldown": ["b"]}}
	}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for applies_to_all_tiers = false, which this code doesn't implement, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsMissingRampUpNote(t *testing.T) {
	bad := baseWarmupCooldownConfigJSON(`{
		"_status": "x",
		"applies_to_all_tiers": true,
		"zones": {"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["a"], "cooldown": ["b"]}}
	}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for a missing ramp_up_note, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsZoneWithEmptyWarmupOrCooldown(t *testing.T) {
	cases := map[string]string{
		"empty warmup":   `{"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": [], "cooldown": ["b"]}`,
		"empty cooldown": `{"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["a"], "cooldown": []}`,
	}
	for name, zoneJSON := range cases {
		bad := baseWarmupCooldownConfigJSON(`{
			"_status": "x",
			"applies_to_all_tiers": true,
			"ramp_up_note": "note",
			"zones": {"Lower body": ` + zoneJSON + `}
		}`)
		if _, err := NewBlockConfigFromJSON(bad); err == nil {
			t.Errorf("%s: expected an error, got nil", name)
		}
	}
}

func TestLoadBlockConfigFromJSON_RejectsZoneWithNonPositiveDuration(t *testing.T) {
	bad := baseWarmupCooldownConfigJSON(`{
		"_status": "x",
		"applies_to_all_tiers": true,
		"ramp_up_note": "note",
		"zones": {"Lower body": {"warmup_minutes": 0, "cooldown_minutes": 5, "warmup": ["a"], "cooldown": ["b"]}}
	}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for warmup_minutes = 0, got nil")
	}
}
