package strengthlibrary

import "fmt"

// exerciseTypeInfo is one entry of VIV_Strength_Block_Config.json's
// exercise_types — which prescription type code an exercise's NAME
// (the key, not its ID) maps to, and whether its prescription text
// needs "per side" appended.
type exerciseTypeInfo struct {
	code    string
	perSide bool
}

// ShortRules is variant_rules.short — how DurationTier=Short trims a
// session: drop slots whose priority appears in DropOrder, trying each
// priority in order, never dropping a priority in NeverDrop, stopping
// once dropping the next slot would take the total below MinExercises.
type ShortRules struct {
	DropOrder    []SlotPriority
	NeverDrop    []SlotPriority
	MinExercises int
}

// IntensityLowRule is variant_rules.intensity_mapping.Low — how an
// Intensity=Low SlotAssignment adjusts an exercise's base prescription
// text: add ExtraRIR to every RIR number present, and reduce the
// leading set count by -SetsDelta (SetsDelta is negative), clamped to
// MinSets.
type IntensityLowRule struct {
	ExtraRIR  int
	SetsDelta int
	MinSets   int
}

// VariantRules is VIV_Strength_Block_Config.json's variant_rules —
// everything session assembly needs to apply the cascade's (VIV-105)
// tier decisions, read from config rather than hardcoded, per the
// task's own instruction.
type VariantRules struct {
	Short ShortRules

	// ReducedLoadUsesWeek1 is true when reduced_load.rule is the one
	// policy this package knows how to apply
	// ("use_week_1_prescription_of_each_exercise_type") — loadBlockConfigFromJSON
	// fails loudly if the config ever asks for a different, unsupported
	// rule rather than silently ignoring it.
	ReducedLoadUsesWeek1 bool

	// SimplifiedActive mirrors variant_rules.simplified.active — wired
	// directly into cascade.ContentLibrary.HasVariant(Strength,
	// VariantSimplified) by whoever constructs that adapter (see
	// internal/core/strengthsession). false today; this package never
	// hardcodes that, only reads it.
	SimplifiedActive bool

	IntensityLow IntensityLowRule
	// Medium and High are both validated at load time to be exactly
	// the two behaviors this package implements ("as written" and
	// "treated as Medium") — see populatePrescriptionConfig — so
	// there's nothing to store here; a config asking for a third
	// behavior fails to load instead of being silently ignored.
}

type rawExerciseType struct {
	Type    string `json:"type"`
	PerSide bool   `json:"per_side"`
}

type rawVariantRules struct {
	Short struct {
		DropOrder    []string `json:"drop_order"`
		NeverDrop    []string `json:"never_drop"`
		MinExercises int      `json:"min_exercises"`
	} `json:"short"`
	ReducedLoad struct {
		Rule string `json:"rule"`
	} `json:"reduced_load"`
	Simplified struct {
		Active bool `json:"active"`
	} `json:"simplified"`
	IntensityMapping struct {
		Low struct {
			ExtraRIR  int `json:"extra_rir"`
			SetsDelta int `json:"sets_delta"`
			MinSets   int `json:"min_sets"`
		} `json:"Low"`
		Medium string `json:"Medium"`
		High   string `json:"High"`
	} `json:"intensity_mapping"`
}

const (
	reducedLoadRuleWeek1         = "use_week_1_prescription_of_each_exercise_type"
	intensityMediumAsWritten     = "as_written"
	intensityHighTreatedAsMedium = "treated_as_Medium_until_reviewed"
)

// populatePrescriptionConfig fills in cfg's status/exerciseTypes/
// prescriptionByType/variantRules from raw, validating everything VIV-112
// needs to exist and be internally consistent — missing/unsupported
// values fail the load instead of defaulting silently.
func populatePrescriptionConfig(cfg *BlockConfig, raw rawBlockConfig) error {
	if raw.Status == "" {
		return fmt.Errorf("strengthlibrary: block config: missing required key %q", "status")
	}
	cfg.status = raw.Status

	if len(raw.WeeklyPrescriptionByType) == 0 {
		return fmt.Errorf("strengthlibrary: block config: missing required key %q", "weekly_prescription_by_type")
	}
	prescriptionByType := make(map[string][4]string, len(raw.WeeklyPrescriptionByType))
	for code, texts := range raw.WeeklyPrescriptionByType {
		if len(texts) != 4 {
			return fmt.Errorf("strengthlibrary: block config: weekly_prescription_by_type[%q] has %d week(s), want exactly 4", code, len(texts))
		}
		for week, text := range texts {
			if text == "" {
				return fmt.Errorf("strengthlibrary: block config: weekly_prescription_by_type[%q] week %d is empty", code, week+1)
			}
		}
		prescriptionByType[code] = [4]string{texts[0], texts[1], texts[2], texts[3]}
	}
	cfg.prescriptionByType = prescriptionByType

	if len(raw.ExerciseTypes) == 0 {
		return fmt.Errorf("strengthlibrary: block config: missing required key %q", "exercise_types")
	}
	exerciseTypes := make(map[string]exerciseTypeInfo, len(raw.ExerciseTypes))
	for name, info := range raw.ExerciseTypes {
		if info.Type == "" {
			return fmt.Errorf("strengthlibrary: block config: exercise_types[%q] has no type", name)
		}
		if _, ok := prescriptionByType[info.Type]; !ok {
			return fmt.Errorf("strengthlibrary: block config: exercise_types[%q] has type %q, which has no weekly_prescription_by_type entry", name, info.Type)
		}
		exerciseTypes[name] = exerciseTypeInfo{code: info.Type, perSide: info.PerSide}
	}
	cfg.exerciseTypes = exerciseTypes

	vr := raw.VariantRules
	if len(vr.Short.DropOrder) == 0 {
		return fmt.Errorf("strengthlibrary: block config: variant_rules.short.drop_order is required")
	}
	if len(vr.Short.NeverDrop) == 0 {
		return fmt.Errorf("strengthlibrary: block config: variant_rules.short.never_drop is required")
	}
	if vr.Short.MinExercises < 1 {
		return fmt.Errorf("strengthlibrary: block config: variant_rules.short.min_exercises must be >= 1, got %d", vr.Short.MinExercises)
	}
	if vr.ReducedLoad.Rule != reducedLoadRuleWeek1 {
		return fmt.Errorf("strengthlibrary: block config: variant_rules.reduced_load.rule %q is not a rule this code implements (only %q)", vr.ReducedLoad.Rule, reducedLoadRuleWeek1)
	}
	if vr.IntensityMapping.Medium != intensityMediumAsWritten {
		return fmt.Errorf("strengthlibrary: block config: variant_rules.intensity_mapping.Medium %q is not a behavior this code implements (only %q)", vr.IntensityMapping.Medium, intensityMediumAsWritten)
	}
	if vr.IntensityMapping.High != intensityHighTreatedAsMedium {
		return fmt.Errorf("strengthlibrary: block config: variant_rules.intensity_mapping.High %q is not a behavior this code implements (only %q)", vr.IntensityMapping.High, intensityHighTreatedAsMedium)
	}
	if vr.IntensityMapping.Low.MinSets < 1 {
		return fmt.Errorf("strengthlibrary: block config: variant_rules.intensity_mapping.Low.min_sets must be >= 1, got %d", vr.IntensityMapping.Low.MinSets)
	}

	dropOrder := make([]SlotPriority, len(vr.Short.DropOrder))
	for i, p := range vr.Short.DropOrder {
		dropOrder[i] = SlotPriority(p)
	}
	neverDrop := make([]SlotPriority, len(vr.Short.NeverDrop))
	for i, p := range vr.Short.NeverDrop {
		neverDrop[i] = SlotPriority(p)
	}

	cfg.variantRules = VariantRules{
		Short: ShortRules{
			DropOrder:    dropOrder,
			NeverDrop:    neverDrop,
			MinExercises: vr.Short.MinExercises,
		},
		ReducedLoadUsesWeek1: true,
		SimplifiedActive:     vr.Simplified.Active,
		IntensityLow: IntensityLowRule{
			ExtraRIR:  vr.IntensityMapping.Low.ExtraRIR,
			SetsDelta: vr.IntensityMapping.Low.SetsDelta,
			MinSets:   vr.IntensityMapping.Low.MinSets,
		},
	}
	return nil
}

// Status is VIV_Strength_Block_Config.json's own status field verbatim
// (e.g. "draft_not_reviewed_by_sport_scientist") — the task requires
// this stay traceable all the way to session-assembly output, never
// silently dropped.
func (c *BlockConfig) Status() string {
	return c.status
}

// TypeOf looks up exerciseName (the exercise's Name, not its ID) in
// exercise_types. ok=false means this exercise has no type configured —
// a hard error for the caller per the task ("an exercise has no type in
// the config" must fail loudly), never a silent default prescription.
func (c *BlockConfig) TypeOf(exerciseName string) (code string, perSide bool, ok bool) {
	info, ok := c.exerciseTypes[exerciseName]
	if !ok {
		return "", false, false
	}
	return info.code, info.perSide, true
}

// PrescriptionText returns typeCode's prescription text for week
// (1-4). Errors on a week outside 1-4, or a typeCode with no
// weekly_prescription_by_type entry (can't happen for a code that came
// from TypeOf, since populatePrescriptionConfig cross-validates every
// exercise_types entry against weekly_prescription_by_type at load
// time — but a caller could still pass an arbitrary string).
func (c *BlockConfig) PrescriptionText(typeCode string, week int) (string, error) {
	if week < 1 || week > 4 {
		return "", fmt.Errorf("strengthlibrary: week %d is outside 1-4", week)
	}
	texts, ok := c.prescriptionByType[typeCode]
	if !ok {
		return "", fmt.Errorf("strengthlibrary: no weekly_prescription_by_type entry for type %q", typeCode)
	}
	return texts[week-1], nil
}

// VariantRules returns the config's variant_rules, parsed and
// validated.
func (c *BlockConfig) VariantRules() VariantRules {
	return c.variantRules
}
