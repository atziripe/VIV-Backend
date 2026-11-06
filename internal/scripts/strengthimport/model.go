// Command strengthimport reads the reviewer workbook for VIV-101's
// Strength movement library ("VIV_Strength_Movement_Groups_Lina.xlsx",
// or a CSV export of each tab — see xlsx.go/csv.go) and regenerates
// internal/core/strengthlibrary/data/exercises.json plus a
// needs_review.json of rows a reviewer flagged "Doesn't belong here?".
//
// Usage:
//
//	go run ./internal/scripts/strengthimport -in <path> -out <exercises.json> -needs-review <needs_review.json>
//
// <path> is either a single .xlsx file (one tab per zone) or a directory
// of per-tab CSV files (one file per zone, named after the tab, e.g.
// "Lower body.csv"). See README.md in this directory for the full
// workbook layout contract.
//
// Imports internal/core/strengthlibrary directly for Exercise/Zone/
// ReviewStatus/DemandLevel/ExerciseID rather than redeclaring parallel
// types here — this importer's only job is to produce exactly the JSON
// that package's loader embeds and validates, so there must be exactly
// one definition of the ID slug algorithm and the JSON shape, not two
// that could silently drift apart.
package main

import (
	"strings"

	"viv/internal/core/strengthlibrary"
)

// zoneForTabName maps a workbook tab/sheet name to a strengthlibrary.Zone.
// Matching is case-insensitive and trims whitespace, but otherwise
// exact — an unrecognized tab name is a hard import error (see
// importWorkbook), never silently skipped or guessed.
func zoneForTabName(tabName string) (strengthlibrary.Zone, bool) {
	switch strings.ToLower(strings.TrimSpace(tabName)) {
	case "lower body", "lower":
		return strengthlibrary.ZoneLower, true
	case "upper body", "upper":
		return strengthlibrary.ZoneUpper, true
	case "full body", "full":
		return strengthlibrary.ZoneFull, true
	case "core":
		return strengthlibrary.ZoneCore, true
	default:
		return "", false
	}
}

// needsReviewEntry is one row the importer refused to import because a
// reviewer flagged it "Doesn't belong here?". A human resolves these,
// the importer never guesses where (or whether) they belong.
type needsReviewEntry struct {
	Zone          strengthlibrary.Zone `json:"zone"`
	MovementGroup string               `json:"movement_group,omitempty"`
	Name          string               `json:"name"`
	Comment       string               `json:"comment,omitempty"`
}

// sheet is one tab's raw cell grid, row-major, 0-indexed — rows[0] is
// the tab's actual row 1, regardless of source format (xlsx or CSV).
// Short rows (fewer cells than the header) are valid; cellAt handles
// the out-of-range case as an empty string.
type sheet struct {
	tabName string
	rows    [][]string
}

func cellAt(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[col])
}

// workbook is every tab read from the source, in source order.
type workbook struct {
	sheets []sheet
}
