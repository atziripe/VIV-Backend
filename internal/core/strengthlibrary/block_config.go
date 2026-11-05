package strengthlibrary

import (
	_ "embed"
	"encoding/json"
	"fmt"
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
// which slots exist per zone, and how each priority tier rotates by
// default.
type BlockConfig struct {
	zones          map[Zone][]Slot
	rotationPolicy map[SlotPriority]rotationSpec
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

type rawBlockConfig struct {
	Zones map[string]struct {
		Slots []rawSlot `json:"slots"`
	} `json:"zones"`
	RotationPolicy map[string]json.RawMessage `json:"rotation_policy"`
}

type rawSlot struct {
	Name                 string   `json:"name"`
	Priority             string   `json:"priority"`
	MovementGroups       []string `json:"movement_groups"`
	RotationPeriodBlocks *int     `json:"rotation_period_blocks,omitempty"`
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

	validZone := make(map[string]bool, len(AllZones))
	for _, z := range AllZones {
		validZone[string(z)] = true
	}

	for zoneName, zoneRaw := range raw.Zones {
		if !validZone[zoneName] {
			return nil, fmt.Errorf("strengthlibrary: block config: unknown zone %q", zoneName)
		}
		zone := Zone(zoneName)
		if len(zoneRaw.Slots) == 0 {
			return nil, fmt.Errorf("strengthlibrary: block config: zone %q has no slots", zoneName)
		}

		seenSlotName := make(map[string]bool, len(zoneRaw.Slots))
		slots := make([]Slot, 0, len(zoneRaw.Slots))
		for _, s := range zoneRaw.Slots {
			if s.Name == "" {
				return nil, fmt.Errorf("strengthlibrary: block config: zone %q has a slot with no name", zoneName)
			}
			if seenSlotName[s.Name] {
				return nil, fmt.Errorf("strengthlibrary: block config: zone %q has a duplicate slot name %q", zoneName, s.Name)
			}
			seenSlotName[s.Name] = true

			priority := SlotPriority(s.Priority)
			if priority != PriorityMain && priority != PriorityAccessory && priority != PriorityOptional {
				return nil, fmt.Errorf("strengthlibrary: block config: zone %q slot %q has unknown priority %q", zoneName, s.Name, s.Priority)
			}
			if len(s.MovementGroups) == 0 {
				return nil, fmt.Errorf("strengthlibrary: block config: zone %q slot %q has no movement_groups", zoneName, s.Name)
			}
			if s.RotationPeriodBlocks != nil && *s.RotationPeriodBlocks < 1 {
				return nil, fmt.Errorf("strengthlibrary: block config: zone %q slot %q has rotation_period_blocks < 1", zoneName, s.Name)
			}

			groups := make([]MovementGroup, len(s.MovementGroups))
			for i, g := range s.MovementGroups {
				groups[i] = MovementGroup(g)
			}
			slots = append(slots, Slot{
				Name:                 s.Name,
				Priority:             priority,
				MovementGroups:       groups,
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
