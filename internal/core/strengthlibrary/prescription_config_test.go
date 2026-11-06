package strengthlibrary

import "testing"

func TestLoadBlockConfig_EmbeddedPrescriptionDataIsValid(t *testing.T) {
	cfg, err := LoadBlockConfig()
	if err != nil {
		t.Fatalf("LoadBlockConfig(): %v", err)
	}
	if cfg.Status() == "" {
		t.Error("expected a non-empty status")
	}
	vr := cfg.VariantRules()
	if vr.Short.MinExercises < 1 {
		t.Errorf("Short.MinExercises = %d, want >= 1", vr.Short.MinExercises)
	}
	if !vr.ReducedLoadUsesWeek1 {
		t.Error("expected ReducedLoadUsesWeek1 = true for the real config")
	}

	code, perSide, ok := cfg.TypeOf("Barbell Back Squat")
	if !ok {
		t.Fatal("expected Barbell Back Squat to have a configured type")
	}
	if code != "C" {
		t.Errorf("Barbell Back Squat type = %q, want C", code)
	}
	if perSide {
		t.Error("Barbell Back Squat should not be per_side")
	}

	text, err := cfg.PrescriptionText(code, 1)
	if err != nil {
		t.Fatalf("PrescriptionText: %v", err)
	}
	if text == "" {
		t.Error("expected non-empty prescription text")
	}
}

func TestBlockConfig_TypeOf_UnknownExerciseReturnsFalse(t *testing.T) {
	cfg, err := LoadBlockConfig()
	if err != nil {
		t.Fatalf("LoadBlockConfig(): %v", err)
	}
	if _, _, ok := cfg.TypeOf("Not A Real Exercise"); ok {
		t.Error("expected ok=false for an exercise with no configured type")
	}
}

func TestBlockConfig_PrescriptionText_WeekOutOfRangeErrors(t *testing.T) {
	cfg, err := LoadBlockConfig()
	if err != nil {
		t.Fatalf("LoadBlockConfig(): %v", err)
	}
	for _, week := range []int{0, 5, -1} {
		if _, err := cfg.PrescriptionText("C", week); err == nil {
			t.Errorf("week %d: expected an error, got nil", week)
		}
	}
}

func TestLoadBlockConfigFromJSON_RejectsMissingStatus(t *testing.T) {
	bad := []byte(`{
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Goblet Squat": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 3},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		}
	}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for a missing status, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsExerciseTypeWithNoWeeklyPrescription(t *testing.T) {
	bad := []byte(`{
		"status": "x",
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Goblet Squat": {"type": "Z", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 3},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		}
	}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for an exercise type with no prescription entry, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsWrongPrescriptionWeekCount(t *testing.T) {
	bad := []byte(`{
		"status": "x",
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Goblet Squat": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["w1", "w2", "w3"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 3},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		}
	}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for a prescription with != 4 weeks, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsUnsupportedReducedLoadRule(t *testing.T) {
	bad := []byte(`{
		"status": "x",
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Goblet Squat": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 3},
			"reduced_load": {"rule": "something_else_entirely"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		}
	}`)
	if _, err := NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for an unsupported reduced_load rule, got nil")
	}
}

func TestLoadBlockConfigFromJSON_RejectsUnsupportedIntensityMediumOrHigh(t *testing.T) {
	cases := map[string]string{"Medium": "something_unexpected", "High": "something_unexpected"}
	for field, badValue := range cases {
		medium, high := "as_written", "treated_as_Medium_until_reviewed"
		if field == "Medium" {
			medium = badValue
		} else {
			high = badValue
		}
		bad := []byte(`{
			"status": "x",
			"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"]}]},
			"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
			"exercise_types": {"Goblet Squat": {"type": "C", "per_side": false}},
			"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
			"variant_rules": {
				"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 3},
				"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
				"simplified": {"active": false},
				"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "` + medium + `", "High": "` + high + `"}
			}
		}`)
		if _, err := NewBlockConfigFromJSON(bad); err == nil {
			t.Fatalf("field=%s: expected an error for an unsupported intensity_mapping value, got nil", field)
		}
	}
}
