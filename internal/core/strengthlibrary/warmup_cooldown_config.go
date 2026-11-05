package strengthlibrary

import "fmt"

// WarmupCooldown is one zone's entry in VIV_Strength_Block_Config.json's
// warmup_cooldown.zones — a fixed, ordered list of plain-text movement
// lines plus a display duration for each of warm-up and cool-down.
// Movements are kept exactly as written: never parsed into sets/reps,
// same convention as weekly_prescription_by_type's prescription text.
type WarmupCooldown struct {
	WarmupMinutes   int
	CooldownMinutes int
	Warmup          []string
	Cooldown        []string
}

type rawWarmupCooldownZone struct {
	WarmupMinutes   int      `json:"warmup_minutes"`
	CooldownMinutes int      `json:"cooldown_minutes"`
	Warmup          []string `json:"warmup"`
	Cooldown        []string `json:"cooldown"`
}

type rawWarmupCooldownConfig struct {
	Status            string                           `json:"_status"`
	AppliesToAllTiers bool                             `json:"applies_to_all_tiers"`
	RampUpNote        string                           `json:"ramp_up_note"`
	Zones             map[string]rawWarmupCooldownZone `json:"zones"`
}

// populateWarmupCooldownConfig fills in cfg's warmup/cooldown data from
// raw.WarmupCooldown, failing loudly if the whole section is missing or
// any zone entry it does list is incomplete — never silently falling
// back to an empty warm-up. applies_to_all_tiers must be true: this
// package only ever implements "every tier gets the full warm-up/
// cool-down, only exercise slots are trimmed" — a config that ever asks
// for the other behavior fails to load rather than being silently
// ignored, same convention as reduced_load.rule/intensity_mapping.
func populateWarmupCooldownConfig(cfg *BlockConfig, raw rawBlockConfig) error {
	wc := raw.WarmupCooldown
	if len(wc.Zones) == 0 {
		return fmt.Errorf("strengthlibrary: block config: missing required key %q", "warmup_cooldown")
	}
	if wc.Status == "" {
		return fmt.Errorf("strengthlibrary: block config: warmup_cooldown is missing required key %q", "_status")
	}
	if !wc.AppliesToAllTiers {
		return fmt.Errorf("strengthlibrary: block config: warmup_cooldown.applies_to_all_tiers = false is not a behavior this code implements (only a full warm-up/cool-down for every tier)")
	}
	if wc.RampUpNote == "" {
		return fmt.Errorf("strengthlibrary: block config: warmup_cooldown is missing required key %q", "ramp_up_note")
	}

	byZone := make(map[Zone]WarmupCooldown, len(wc.Zones))
	for zoneKey, z := range wc.Zones {
		zone, ok := zoneFromConfigKey(zoneKey)
		if !ok {
			return fmt.Errorf("strengthlibrary: block config: warmup_cooldown.zones has unknown zone %q", zoneKey)
		}
		if len(z.Warmup) == 0 {
			return fmt.Errorf("strengthlibrary: block config: warmup_cooldown.zones[%q] has no warmup movements", zoneKey)
		}
		if len(z.Cooldown) == 0 {
			return fmt.Errorf("strengthlibrary: block config: warmup_cooldown.zones[%q] has no cooldown movements", zoneKey)
		}
		if z.WarmupMinutes < 1 {
			return fmt.Errorf("strengthlibrary: block config: warmup_cooldown.zones[%q].warmup_minutes must be >= 1, got %d", zoneKey, z.WarmupMinutes)
		}
		if z.CooldownMinutes < 1 {
			return fmt.Errorf("strengthlibrary: block config: warmup_cooldown.zones[%q].cooldown_minutes must be >= 1, got %d", zoneKey, z.CooldownMinutes)
		}
		byZone[zone] = WarmupCooldown{
			WarmupMinutes:   z.WarmupMinutes,
			CooldownMinutes: z.CooldownMinutes,
			Warmup:          z.Warmup,
			Cooldown:        z.Cooldown,
		}
	}

	cfg.warmupCooldownStatus = wc.Status
	cfg.rampUpNote = wc.RampUpNote
	cfg.warmupCooldownByZone = byZone
	return nil
}

// WarmupCooldownStatus is warmup_cooldown._status verbatim — the task
// requires this stay traceable next to the existing config Status(),
// never silently dropped.
func (c *BlockConfig) WarmupCooldownStatus() string {
	return c.warmupCooldownStatus
}

// RampUpNote is warmup_cooldown.ramp_up_note, shown once per session
// before the first Main exercise — never per-zone, never repeated per
// exercise.
func (c *BlockConfig) RampUpNote() string {
	return c.rampUpNote
}

// WarmupCooldownForZone returns zone's configured warm-up/cool-down.
// ok=false means this zone has no entry at all — a hard error for the
// caller (strengthsession.Assemble): never fall back to an empty
// warm-up.
func (c *BlockConfig) WarmupCooldownForZone(zone Zone) (WarmupCooldown, bool) {
	wc, ok := c.warmupCooldownByZone[zone]
	return wc, ok
}
