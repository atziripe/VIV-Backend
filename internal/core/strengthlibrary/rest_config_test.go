package strengthlibrary

import "testing"

func TestLoadBlockConfig_EmbeddedRestSecondsIsValid(t *testing.T) {
	cfg, err := LoadBlockConfig()
	if err != nil {
		t.Fatalf("LoadBlockConfig(): %v", err)
	}
	if cfg.RestSecondsStatus() == "" {
		t.Error("expected a non-empty RestSecondsStatus")
	}

	code, _, ok := cfg.TypeOf("Barbell Back Squat")
	if !ok {
		t.Fatal("expected Barbell Back Squat to have a configured type")
	}
	for week := 1; week <= 4; week++ {
		seconds, err := cfg.RestSeconds(code, week)
		if err != nil {
			t.Fatalf("RestSeconds(%q, %d): %v", code, week, err)
		}
		if seconds < 1 {
			t.Errorf("RestSeconds(%q, %d) = %d, want >= 1", code, week, seconds)
		}
	}
}

func TestBlockConfig_RestSeconds_WeekOutOfRangeErrors(t *testing.T) {
	cfg, err := LoadBlockConfig()
	if err != nil {
		t.Fatalf("LoadBlockConfig(): %v", err)
	}
	for _, week := range []int{0, 5, -1} {
		if _, err := cfg.RestSeconds("C", week); err == nil {
			t.Errorf("week %d: expected an error, got nil", week)
		}
	}
}

func TestBlockConfig_RestSeconds_UnknownTypeErrors(t *testing.T) {
	cfg, err := LoadBlockConfig()
	if err != nil {
		t.Fatalf("LoadBlockConfig(): %v", err)
	}
	if _, err := cfg.RestSeconds("Not A Real Type", 1); err == nil {
		t.Error("expected an error for a type with no rest_seconds_by_type entry, got nil")
	}
}

// baseRestSecondsConfigJSON builds a full, otherwise-valid config (status,
// zones, exercise_types, weekly_prescription_by_type, variant_rules,
// warmup_cooldown all present and passing their own validation) with
// restSecondsOverride spliced in as the rest_seconds_by_type section — so
// a test can isolate a rest_seconds_by_type-specific failure from every
// other required section.
func baseRestSecondsConfigJSON(restSecondsOverride string) []byte {
	return []byte(`{
		"status": "x",
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"], "pool_order": ["Goblet Squat"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Goblet Squat": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 1},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		},
		"warmup_cooldown": {
			"_status": "x",
			"applies_to_all_tiers": true,
			"ramp_up_note": "note",
			"zones": {"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["a"], "cooldown": ["b"]}}
		},
		"rest_seconds_by_type": ` + restSecondsOverride + `
	}`)
}

func TestLoadBlockConfigFromJSON_RejectsMissingRestSecondsByType(t *testing.T) {
	bad := []byte(`{
		"status": "x",
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"], "pool_order": ["Goblet Squat"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Goblet Squat": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 1},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		},
		"warmup_cooldown": {
			"_status": "x",
			"applies_to_all_tiers": true,
			"ramp_up_note": "note",
			"zones": {"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["a"], "cooldown": ["b"]}}
		}
	}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for a config with no rest_seconds_by_type section at all, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsMissingRestSecondsStatus(t *testing.T) {
	bad := baseRestSecondsConfigJSON(`{"C": [90, 90, 120, 120]}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for rest_seconds_by_type with no _status, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsExerciseTypeWithNoRestSecondsEntry(t *testing.T) {
	bad := baseRestSecondsConfigJSON(`{"_status": "x", "Z": [90, 90, 120, 120]}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for an exercise type (C) with no rest_seconds_by_type entry, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsWrongRestSecondsWeekCount(t *testing.T) {
	bad := baseRestSecondsConfigJSON(`{"_status": "x", "C": [90, 90, 120]}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for a rest_seconds_by_type entry with != 4 weeks, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsNonPositiveRestSeconds(t *testing.T) {
	bad := baseRestSecondsConfigJSON(`{"_status": "x", "C": [90, 0, 120, 120]}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for a rest_seconds_by_type entry with a non-positive value, got nil")
	}
}

func TestLoadBlockConfigFromJSON_AcceptsValidRestSecondsByType(t *testing.T) {
	good := baseRestSecondsConfigJSON(`{"_status": "x", "C": [90, 90, 120, 120]}`)
	cfg, err := NewBlockConfigFromJSON(good)
	if err != nil {
		t.Fatalf("NewBlockConfigFromJSON: %v", err)
	}
	if cfg.RestSecondsStatus() != "x" {
		t.Errorf("RestSecondsStatus() = %q, want %q", cfg.RestSecondsStatus(), "x")
	}
	seconds, err := cfg.RestSeconds("C", 3)
	if err != nil {
		t.Fatalf("RestSeconds: %v", err)
	}
	if seconds != 120 {
		t.Errorf("RestSeconds(C, 3) = %d, want 120", seconds)
	}
}
