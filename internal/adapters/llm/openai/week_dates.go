package openai

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func extractWeekDates(trainingJSON []byte) (weekStart string, weekEnd string, err error) {
	var m map[string]any
	if err := json.Unmarshal(trainingJSON, &m); err != nil {
		return "", "", fmt.Errorf("training payload is not valid JSON: %w", err)
	}

	getStr := func(v any) string {
		s, _ := v.(string)
		return strings.TrimSpace(s)
	}

	// 1) root week_start/week_end
	ws := getStr(m["week_start"])
	we := getStr(m["week_end"])
	if ws != "" && we != "" {
		return ws, we, nil
	}

	// 2) root start_date/end_date (alias)
	ws = getStr(m["start_date"])
	we = getStr(m["end_date"])
	if ws != "" && we != "" {
		return ws, we, nil
	}

	// 3) meta.* variants
	if meta, ok := m["meta"].(map[string]any); ok {
		ws = getStr(meta["week_start"])
		we = getStr(meta["week_end"])
		if ws != "" && we != "" {
			return ws, we, nil
		}

		ws = getStr(meta["start_date"])
		we = getStr(meta["end_date"])
		if ws != "" && we != "" {
			return ws, we, nil
		}
	}

	return "", "", fmt.Errorf("training week_start/week_end missing")
}

// Computes a 7-day window starting today (UTC, 00:00)
func computeWeekRangeUTC(now time.Time) (start time.Time, end time.Time) {
	n := now.UTC()
	start = time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 0, 6)
	return start, end
}

func resolveWeekDates(trainingJSON []byte, now time.Time) (start time.Time, end time.Time, usedFallback bool) {
	ws, we, err := extractWeekDates(trainingJSON)
	if err != nil {
		start, end = computeWeekRangeUTC(now)
		return start, end, true
	}

	parsedStart, err1 := time.Parse("2006-01-02", ws)
	parsedEnd, err2 := time.Parse("2006-01-02", we)
	if err1 != nil || err2 != nil {
		start, end = computeWeekRangeUTC(now)
		return start, end, true
	}

	start = time.Date(parsedStart.Year(), parsedStart.Month(), parsedStart.Day(), 0, 0, 0, 0, time.UTC)
	end = time.Date(parsedEnd.Year(), parsedEnd.Month(), parsedEnd.Day(), 0, 0, 0, 0, time.UTC)
	return start, end, false
}
