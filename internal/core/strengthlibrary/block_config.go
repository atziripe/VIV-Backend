package strengthlibrary

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// SlotPriority is how important a slot is within a zone's block —
// drives the DEFAULT rotation behavior (see BlockConfig's
// rotation_policy): Main slots hold the same exercise for the whole
// mesocycle, Accessory/Optional slots rotate.
type SlotPriority string

const (
	PriorityMain      SlotPriority = "Main"
	PriorityAccessory SlotPriority = "Accessory"
	PriorityOptional  SlotPriority = "Optional"
)

// Slot is one unit of exercise selection within a zone — NOT the same
// thing as a MovementGroup, because a slot can merge several groups
// into one shared candidate pool (e.g. Full body's "Pull" slot merges
// "Vertical pull" and "Horizontal pull" — see VIV_Strength_Block_Config.json).
type Slot struct {
	Name           string
	Priority       SlotPriority
	MovementGroups []MovementGroup

	// PoolOrder is this slot's own explicit candidate order — exercise
	// NAMEs, in the exact order the config author (not this package)
	// wants them tried. BuildSlotPool resolves each name against the
	// zone's approved exercises and never re-sorts this list by any
	// rating or heuristic — see BuildSlotPool's doc comment.
	PoolOrder []string

	// RotationPeriodBlocks overrides BlockConfig's priority-level
	// rotation_policy default for this slot alone — nil means "use the
	// priority default". A non-nil value always means "rotate every N
	// blocks", even for a Main-priority slot; there is no override that
	// means "hold" for a slot whose priority rotates by default, since
	// the task never asked for that direction and it would be an easy
	// footgun to invent silently.
	RotationPeriodBlocks *int
}

// rotationSpec is one priority tier's resolved rotation behavior —
// either "hold forever" (Main) or "rotate every N blocks" (Accessory/
// Optional, N usually 1).
type rotationSpec struct {
	hold         bool
	periodBlocks int
}

// BlockConfig is VIV_Strength_Block_Config.json, loaded and validated —
// which slots exist per zone, how each priority tier rotates by
// default, and (see prescription_config.go) the exercise-type/
// prescription/variant-rules data VIV-112's session assembly needs.
type BlockConfig struct {
	zones          map[Zone][]Slot
	rotationPolicy map[SlotPriority]rotationSpec

	status             string
	exerciseTypes      map[string]exerciseTypeInfo
	prescriptionByType map[string][4]string
	variantRules       VariantRules

	warmupCooldownStatus string
	rampUpNote           string
	warmupCooldownByZone map[Zone]WarmupCooldown

	restSecondsStatus string
	restSecondsByType map[string][4]int
}

//go:embed data/VIV_Strength_Block_Config.json
var blockConfigJSON []byte

// LoadBlockConfig parses and validates the embedded block config. Fails
// fast on anything structurally wrong — an unknown zone/priority name, a
// slot referencing no movement groups, or a rotation_policy value that's
// neither "hold" nor a positive integer.
func LoadBlockConfig() (*BlockConfig, error) {
	return loadBlockConfigFromJSON(blockConfigJSON)
}

// NewBlockConfigFromJSON builds a BlockConfig from arbitrary JSON — for
// other packages' tests that need a small fixture config instead of the
// real one. Production code always uses LoadBlockConfig.
func NewBlockConfigFromJSON(data []byte) (*BlockConfig, error) {
	return loadBlockConfigFromJSON(data)
}

// rawBlockConfig mirrors VIV_Strength_Block_Config.json's real shape —
// each zone is a bare array of slots (not {"slots": [...]}), a slot's
// name/group-list keys are "slot"/"source_groups", and zone keys are
// the long, human-readable names ("Lower body") matching the reviewer
// workbook's own tab names, not this package's short Zone values
// ("lower") — see zoneFromConfigKey. The file also carries
// status/week_4/exercise_types/type_names/weekly_prescription_by_type/
// rir_note — all prescription/session-assembly concerns for a later
// ticket, deliberately not modeled here; encoding/json ignores unknown
// fields by default, so they just pass through unparsed.
type rawBlockConfig struct {
	Status                   string                     `json:"status"`
	Zones                    map[string][]rawSlot       `json:"zones"`
	RotationPolicy           map[string]json.RawMessage `json:"rotation_policy"`
	ExerciseTypes            map[string]rawExerciseType `json:"exercise_types"`
	WeeklyPrescriptionByType map[string][]string        `json:"weekly_prescription_by_type"`
	VariantRules             rawVariantRules            `json:"variant_rules"`
	WarmupCooldown           rawWarmupCooldownConfig    `json:"warmup_cooldown"`
	RestSecondsByType        map[string]json.RawMessage `json:"rest_seconds_by_type"`
}

type rawSlot struct {
	Name                 string   `json:"slot"`
	Priority             string   `json:"priority"`
	MovementGroups       []string `json:"source_groups"`
	PoolOrder            []string `json:"pool_order"`
	RotationPeriodBlocks *int     `json:"rotation_period_blocks,omitempty"`
}

// zoneFromConfigKey maps the config's long zone names to this package's
// short Zone values — case-insensitive, trimmed, same leniency as
// internal/scripts/strengthimport's zoneForTabName, which this
// deliberately mirrors (both ultimately key off the same reviewer
// workbook tab names).
func zoneFromConfigKey(key string) (Zone, bool) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "lower body", "lower":
		return ZoneLower, true
	case "upper body", "upper":
		return ZoneUpper, true
	case "full body", "full":
		return ZoneFull, true
	case "core":
		return ZoneCore, true
	default:
		return "", false
	}
}

func loadBlockConfigFromJSON(data []byte) (*BlockConfig, error) {
	var raw rawBlockConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("strengthlibrary: parsing block config: %w", err)
	}

	cfg := &BlockConfig{
		zones:          make(map[Zone][]Slot, len(raw.Zones)),
		rotationPolicy: make(map[SlotPriority]rotationSpec, len(raw.RotationPolicy)),
	}

	for zoneKey, rawSlots := range raw.Zones {
		zone, ok := zoneFromConfigKey(zoneKey)
		if !ok {
			return nil, fmt.Errorf("strengthlibrary: block config: unknown zone %q", zoneKey)
		}
		if len(rawSlots) == 0 {
			return nil, fmt.Errorf("strengthlibrary: block config: zone %q has no slots", zoneKey)
		}

		seenSlotName := make(map[string]bool, len(rawSlots))
		slots := make([]Slot, 0, len(rawSlots))
		for _, s := range rawSlots {
			if s.Name == "" {
				return nil, fmt.Errorf("strengthlibrary: block config: zone %q has a slot with no name", zoneKey)
			}
			if seenSlotName[s.Name] {
				return nil, fmt.Errorf("strengthlibrary: block config: zone %q has a duplicate slot name %q", zoneKey, s.Name)
			}
			seenSlotName[s.Name] = true

			priority := SlotPriority(s.Priority)
			if priority != PriorityMain && priority != PriorityAccessory && priority != PriorityOptional {
				return nil, fmt.Errorf("strengthlibrary: block config: zone %q slot %q has unknown priority %q", zoneKey, s.Name, s.Priority)
			}
			if len(s.MovementGroups) == 0 {
				return nil, fmt.Errorf("strengthlibrary: block config: zone %q slot %q has no source_groups", zoneKey, s.Name)
			}
			if len(s.PoolOrder) == 0 {
				return nil, fmt.Errorf("strengthlibrary: block config: zone %q slot %q has no pool_order", zoneKey, s.Name)
			}
			if s.RotationPeriodBlocks != nil && *s.RotationPeriodBlocks < 1 {
				return nil, fmt.Errorf("strengthlibrary: block config: zone %q slot %q has rotation_period_blocks < 1", zoneKey, s.Name)
			}

			groups := make([]MovementGroup, len(s.MovementGroups))
			for i, g := range s.MovementGroups {
				groups[i] = MovementGroup(g)
			}
			slots = append(slots, Slot{
				Name:                 s.Name,
				Priority:             priority,
				MovementGroups:       groups,
				PoolOrder:            s.PoolOrder,
				RotationPeriodBlocks: s.RotationPeriodBlocks,
			})
		}
		cfg.zones[zone] = slots
	}

	for _, p := range []SlotPriority{PriorityMain, PriorityAccessory, PriorityOptional} {
		raw, ok := raw.RotationPolicy[string(p)]
		if !ok {
			return nil, fmt.Errorf("strengthlibrary: block config: rotation_policy is missing priority %q", p)
		}
		spec, err := parseRotationSpec(raw)
		if err != nil {
			return nil, fmt.Errorf("strengthlibrary: block config: rotation_policy[%q]: %w", p, err)
		}
		cfg.rotationPolicy[p] = spec
	}

	if err := populatePrescriptionConfig(cfg, raw); err != nil {
		return nil, err
	}

	if err := populateWarmupCooldownConfig(cfg, raw); err != nil {
		return nil, err
	}

	if err := populateRestSecondsConfig(cfg, raw); err != nil {
		return nil, err
	}

	return cfg, nil
}

// parseRotationSpec accepts either the literal string "hold" or a
// positive JSON integer (rotate every N blocks).
func parseRotationSpec(raw json.RawMessage) (rotationSpec, error) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if asString != "hold" {
			return rotationSpec{}, fmt.Errorf("string value must be \"hold\", got %q", asString)
		}
		return rotationSpec{hold: true}, nil
	}

	var asInt int
	if err := json.Unmarshal(raw, &asInt); err == nil {
		if asInt < 1 {
			return rotationSpec{}, fmt.Errorf("integer value must be >= 1, got %d", asInt)
		}
		return rotationSpec{periodBlocks: asInt}, nil
	}

	return rotationSpec{}, fmt.Errorf("value must be \"hold\" or a positive integer, got %s", string(raw))
}

// SlotsForZone returns zone's configured slots, in the config's own
// order — callers must not re-sort it (the task's own ordering rule).
func (c *BlockConfig) SlotsForZone(zone Zone) []Slot {
	return c.zones[zone]
}

// HoldsForever reports whether slot never rotates — true for a
// Main-priority slot with no override; an override is always a rotation
// period, never a hold, so it's always false when RotationPeriodBlocks
// is set.
func (c *BlockConfig) HoldsForever(slot Slot) bool {
	if slot.RotationPeriodBlocks != nil {
		return false
	}
	return c.rotationPolicy[slot.Priority].hold
}

// RotationPeriodBlocks returns how many blocks slot keeps its exercise
// before advancing to the next one in its pool. Meaningless (but safe —
// returns 1) when HoldsForever(slot) is true.
func (c *BlockConfig) RotationPeriodBlocks(slot Slot) int {
	if slot.RotationPeriodBlocks != nil {
		return *slot.RotationPeriodBlocks
	}
	if spec := c.rotationPolicy[slot.Priority]; spec.periodBlocks >= 1 {
		return spec.periodBlocks
	}
	return 1
}
