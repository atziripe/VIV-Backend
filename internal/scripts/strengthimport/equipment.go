package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// controlledEquipmentTags is the fixed, authoritative vocabulary —
// defined here in Go, not in the mapping file, so the mapping file can
// never introduce a tag nobody agreed to. loadEquipmentMapping validates
// every mapped value against this set.
var controlledEquipmentTags = map[string]bool{
	"none": true, "mat": true, "dumbbell": true, "kettlebell": true,
	"barbell": true, "bench": true, "box": true, "rack": true,
	"machine": true, "hand_support": true,
	// Added once the real reviewer workbook showed these are genuinely
	// distinct equipment, not a fit for any of the original 10 — see
	// the commit that introduced them for the full reasoning.
	"ab_wheel": true, "resistance_band": true, "weighted_vest": true,
	"pull_up_bar": true, "dip_bars": true,
}

// loadEquipmentMapping reads the free-text -> controlled-tag JSON file
// at path. Keys are normalized (lowercased, trimmed) on load so lookups
// in mapEquipment don't have to re-normalize every call.
func loadEquipmentMapping(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading equipment mapping %s: %w", path, err)
	}

	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing equipment mapping %s: %w", path, err)
	}

	mapping := make(map[string]string, len(raw))
	for k, v := range raw {
		// A value is normally one tag, but a single un-splittable
		// phrase can legitimately need two (e.g. "bench and rack with
		// safety arms" needs both bench AND rack with no comma for
		// mapEquipment's splitter to key off) — the mapping file
		// expresses that as a comma-separated value, validated tag by
		// tag below.
		for _, part := range strings.Split(v, ",") {
			tag := strings.ToLower(strings.TrimSpace(part))
			// "" is a deliberate third outcome, distinct from both a
			// real tag and "unmapped" — see mapEquipment's doc comment
			// for what it's for (an attachment/optional-accessory
			// phrase that legitimately contributes no required tag).
			if tag != "" && !controlledEquipmentTags[tag] {
				return nil, fmt.Errorf(
					"equipment mapping %s: %q maps to %q, which is not a controlled tag — "+
						"add it to controlledEquipmentTags first if this is intentional",
					path, k, part,
				)
			}
		}
		mapping[strings.ToLower(strings.TrimSpace(k))] = strings.ToLower(strings.TrimSpace(v))
	}
	return mapping, nil
}

// mapEquipment splits a workbook "Equipment required" cell into pieces
// and maps each one through mapping. Sheets aren't consistent about
// separators for a multi-equipment cell — comma and semicolon separate
// distinct requirements, and so does "+" (the real workbook's own
// convention for "this AND that", e.g. "Dumbbell + stable bench").
// "or" is deliberately NOT split on: it signals true alternatives
// (either works), which this flat AND-list model can't represent as
// two tags — those phrases get one literal mapping entry instead (see
// equipment_mapping.json's comments on which tag was picked and why).
//
// A mapped value of "" means the phrase is real but contributes no
// required tag — a pure attachment/descriptor ("handle" on a cable
// machine already captured as "machine", "clear walking space", "two
// stable" as a sentence fragment split off from "...non-rolling
// dumbbells"). "none" instead means the phrase itself asserts "nothing
// is really required" ("mat optional", "padding optional", "light
// dumbbells optional") — it's a real tag, so it appears in the output,
// UNLESS the same cell also produced a genuine equipment tag, in which
// case asserting both "needs X" and "needs nothing" is contradictory
// and "none" is dropped (see the dedup loop below).
//
// tags is deduplicated, in first-seen order. unmapped lists every
// piece with NO mapping entry at all — never guessed, always surfaced
// so the caller can fail the import and report it.
func mapEquipment(raw string, mapping map[string]string) (tags []string, unmapped []string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	replaced := strings.NewReplacer(";", ",", "\n", ",", "+", ",").Replace(raw)
	seen := make(map[string]bool)
	for _, piece := range strings.Split(replaced, ",") {
		piece = strings.TrimSpace(piece)
		if piece == "" {
			continue
		}
		mapped, ok := mapping[strings.ToLower(piece)]
		if !ok {
			unmapped = append(unmapped, piece)
			continue
		}
		for _, tag := range strings.Split(mapped, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue // a recognized phrase/sub-tag that contributes nothing
			}
			if !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}

	if len(tags) > 1 {
		filtered := tags[:0]
		for _, t := range tags {
			if t != "none" {
				filtered = append(filtered, t)
			}
		}
		tags = filtered
	}
	return tags, unmapped
}
