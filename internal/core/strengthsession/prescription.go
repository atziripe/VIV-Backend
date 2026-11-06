package strengthsession

import (
	"fmt"
	"regexp"
	"strconv"

	"viv/internal/core/strengthlibrary"
)

// leadingSetsPattern matches the leading "N ×" every
// weekly_prescription_by_type entry starts with (verified against the
// real config's full 8 type codes — every single entry, regardless of
// format, opens this way).
var leadingSetsPattern = regexp.MustCompile(`^(\d+)(\s*×)`)

// rirPattern matches "RIR <n>" or "RIR <n>–<m>" wherever it appears —
// only type codes C/U/B/A ever carry it; P/K/T/Cr prescriptions have no
// RIR at all, which is fine: the replace below is simply a no-op for
// those, applyLowIntensity still adjusts the leading set count.
var rirPattern = regexp.MustCompile(`RIR\s*(\d+)(–(\d+))?`)

// applyLowIntensity rewrites a base prescription text per
// variant_rules.intensity_mapping.Low: reduce the leading set count by
// -rule.SetsDelta (clamped to rule.MinSets), and add rule.ExtraRIR to
// every RIR number present (both ends of a range, if there is one).
// Everything else in the text — rep ranges, tempo/load notes — is left
// exactly as written, per the task's "keep the text as written" rule
// for the base lookup; this is the one explicitly-specified exception,
// using the config's own structured numbers, never an invented value.
//
// Errors (rather than silently returning the text unchanged) if it
// doesn't even have the universal leading "N ×" — every real
// weekly_prescription_by_type entry has one, so this should be
// unreachable against real data; it exists so a future malformed
// config entry fails loudly instead of silently skipping the
// adjustment.
// parseLeadingSetCount reads the leading "N" off a prescription text's
// universal "N ×" opening — the same number Assemble exposes as
// AssembledExercise.Sets, so a client knows how many set rows to render
// (and LogSetUseCase has something to validate SetNumber against)
// without parsing the display text itself.
func parseLeadingSetCount(text string) (int, error) {
	m := leadingSetsPattern.FindStringSubmatch(text)
	if m == nil {
		return 0, fmt.Errorf("prescription text %q has no leading \"N ×\" set count", text)
	}
	sets, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("parsing set count from %q: %w", text, err)
	}
	return sets, nil
}

func applyLowIntensity(text string, rule strengthlibrary.IntensityLowRule) (string, error) {
	sets, err := parseLeadingSetCount(text)
	if err != nil {
		return "", err
	}
	newSets := sets + rule.SetsDelta
	if newSets < rule.MinSets {
		newSets = rule.MinSets
	}
	text = leadingSetsPattern.ReplaceAllString(text, strconv.Itoa(newSets)+"$2")

	text = rirPattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := rirPattern.FindStringSubmatch(match)
		low, err := strconv.Atoi(sub[1])
		if err != nil {
			return match // leave an unparseable match untouched rather than corrupting it
		}
		out := "RIR " + strconv.Itoa(low+rule.ExtraRIR)
		if sub[3] != "" {
			high, err := strconv.Atoi(sub[3])
			if err != nil {
				return match
			}
			out += "–" + strconv.Itoa(high+rule.ExtraRIR)
		}
		return out
	})

	return text, nil
}
