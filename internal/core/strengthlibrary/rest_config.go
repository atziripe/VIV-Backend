package strengthlibrary

import (
	"encoding/json"
	"fmt"
)

// populateRestSecondsConfig fills in cfg's rest-seconds data from
// raw.RestSecondsByType, failing loudly if the whole section is missing
// or any exercise_types code it references has no rest entry — never
// silently returning 0. Must run after populatePrescriptionConfig, since
// the cross-validation below needs cfg.exerciseTypes already populated.
func populateRestSecondsConfig(cfg *BlockConfig, raw rawBlockConfig) error {
	if len(raw.RestSecondsByType) == 0 {
		return fmt.Errorf("strengthlibrary: block config: missing required key %q", "rest_seconds_by_type")
	}

	rawStatus, ok := raw.RestSecondsByType["_status"]
	if !ok {
		return fmt.Errorf("strengthlibrary: block config: rest_seconds_by_type is missing required key %q", "_status")
	}
	var status string
	if err := json.Unmarshal(rawStatus, &status); err != nil {
		return fmt.Errorf("strengthlibrary: block config: rest_seconds_by_type._status: %w", err)
	}
	if status == "" {
		return fmt.Errorf("strengthlibrary: block config: rest_seconds_by_type is missing required key %q", "_status")
	}

	byType := make(map[string][4]int, len(raw.RestSecondsByType))
	for code, rawValue := range raw.RestSecondsByType {
		if len(code) > 0 && code[0] == '_' {
			continue
		}
		var secondsList []int
		if err := json.Unmarshal(rawValue, &secondsList); err != nil {
			return fmt.Errorf("strengthlibrary: block config: rest_seconds_by_type[%q]: %w", code, err)
		}
		if len(secondsList) != 4 {
			return fmt.Errorf("strengthlibrary: block config: rest_seconds_by_type[%q] has %d week(s), want exactly 4", code, len(secondsList))
		}
		for week, seconds := range secondsList {
			if seconds < 1 {
				return fmt.Errorf("strengthlibrary: block config: rest_seconds_by_type[%q] week %d must be >= 1, got %d", code, week+1, seconds)
			}
		}
		byType[code] = [4]int{secondsList[0], secondsList[1], secondsList[2], secondsList[3]}
	}

	for name, info := range cfg.exerciseTypes {
		if _, ok := byType[info.code]; !ok {
			return fmt.Errorf("strengthlibrary: block config: exercise_types[%q] has type %q, which has no rest_seconds_by_type entry", name, info.code)
		}
	}

	cfg.restSecondsStatus = status
	cfg.restSecondsByType = byType
	return nil
}

// RestSecondsStatus is rest_seconds_by_type._status verbatim — the task
// requires this stay traceable next to the other config status fields,
// never silently dropped.
func (c *BlockConfig) RestSecondsStatus() string {
	return c.restSecondsStatus
}

// RestSeconds returns typeCode's rest time, in seconds, for week (1-4).
// Errors on a week outside 1-4, or a typeCode with no rest_seconds_by_type
// entry (can't happen for a code that came from TypeOf, since
// populateRestSecondsConfig cross-validates every exercise_types entry
// against rest_seconds_by_type at load time — but a caller could still
// pass an arbitrary string). Never returns a silent 0.
func (c *BlockConfig) RestSeconds(typeCode string, week int) (int, error) {
	if week < 1 || week > 4 {
		return 0, fmt.Errorf("strengthlibrary: week %d is outside 1-4", week)
	}
	seconds, ok := c.restSecondsByType[typeCode]
	if !ok {
		return 0, fmt.Errorf("strengthlibrary: no rest_seconds_by_type entry for type %q", typeCode)
	}
	return seconds[week-1], nil
}
