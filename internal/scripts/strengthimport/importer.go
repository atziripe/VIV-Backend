package main

import (
	"fmt"
	"sort"
	"strings"

	"viv/internal/core/strengthlibrary"
)

// parseDemand validates and normalizes a "Technical demand"/"Balance
// demand" cell. A blank cell is allowed (the sheet is a living
// document — not every row has to have these filled in yet) and
// returns ("", true). Anything non-blank that isn't Low/Moderate/High
// (case-insensitive) is rejected — copied-from-the-sheet fidelity does
// not extend to silently accepting a typo.
func parseDemand(raw string) (strengthlibrary.DemandLevel, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return "", true
	case "low":
		return strengthlibrary.DemandLow, true
	case "moderate":
		return strengthlibrary.DemandModerate, true
	case "high":
		return strengthlibrary.DemandHigh, true
	default:
		return "", false
	}
}

// rowRef locates one source row for an error message.
type rowRef struct {
	tabName string
	rowNum  int // 1-indexed, matches what a reviewer sees opening the sheet
	group   string
	name    string
}

func (r rowRef) String() string {
	return fmt.Sprintf("tab %q row %d (%q / %q)", r.tabName, r.rowNum, r.group, r.name)
}

// importResult is everything one importWorkbook call produces.
type importResult struct {
	exercises   []strengthlibrary.Exercise
	needsReview []needsReviewEntry
	errs        []string // non-empty means the import failed; nothing should be written
}

// importWorkbook is the whole transform: raw sheets -> validated,
// deterministic exercise/needs-review sets. Every validation failure is
// collected (not returned on the first one) so a single run surfaces
// everything wrong with the sheet at once — a bulk import is far more
// useful that way than stopping at row one's problem.
func importWorkbook(wb workbook, equipMapping map[string]string) importResult {
	var (
		exercises   []strengthlibrary.Exercise
		needsReview []needsReviewEntry
		errs        []string

		seenTriple = make(map[string]rowRef) // zone|group|name (lowercased) -> first row that claimed it
		seenID     = make(map[string]rowRef) // exercise ID -> first row that produced it
	)

	for _, sh := range wb.sheets {
		z, ok := zoneForTabName(sh.tabName)
		if !ok {
			errs = append(errs, fmt.Sprintf("unknown zone/tab name %q", sh.tabName))
			continue
		}

		headerRow, cols, err := findHeaderRow(sh)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}

		for rowIdx := headerRow + 1; rowIdx < len(sh.rows); rowIdx++ {
			row := sh.rows[rowIdx]
			excelRow := rowIdx + 1

			name := cellAt(row, cols.exercise)
			if name == "" {
				continue // blank row left for additions — skip silently, per spec
			}

			group := cellAt(row, cols.movementGroup)
			ref := rowRef{tabName: sh.tabName, rowNum: excelRow, group: group, name: name}

			var doesntBelong string
			if cols.doesntBelong >= 0 {
				doesntBelong = cellAt(row, cols.doesntBelong)
			}
			if doesntBelong != "" {
				comment := doesntBelong
				if cols.comment >= 0 {
					if c := cellAt(row, cols.comment); c != "" {
						comment = c
					}
				}
				needsReview = append(needsReview, needsReviewEntry{
					Zone:          z,
					MovementGroup: group,
					Name:          name,
					Comment:       comment,
				})
				continue
			}

			if group == "" {
				errs = append(errs, fmt.Sprintf("%s: empty movement group", ref))
				continue
			}

			tripleKey := string(z) + "|" + strings.ToLower(strings.TrimSpace(group)) + "|" + strings.ToLower(strings.TrimSpace(name))
			if existing, dup := seenTriple[tripleKey]; dup {
				errs = append(errs, fmt.Sprintf(
					"duplicate (zone, group, name): %s duplicates %s", ref, existing,
				))
				continue
			}
			seenTriple[tripleKey] = ref

			id := strengthlibrary.ExerciseID(z, name)
			if existing, dup := seenID[id]; dup {
				errs = append(errs, fmt.Sprintf(
					"%s produces the same exercise id %q as %s — same name in the same zone under two different "+
						"movement groups isn't supported; fix the sheet or rename one of them",
					ref, id, existing,
				))
				continue
			}
			seenID[id] = ref

			equipRaw := cellAt(row, cols.equipment)
			tags, unmapped := mapEquipment(equipRaw, equipMapping)
			for _, u := range unmapped {
				errs = append(errs, fmt.Sprintf("%s: equipment %q has no mapping in equipment_mapping.json — add it, never guessed", ref, u))
			}

			technicalDemand, ok := parseDemand(cellAt(row, cols.technicalDemand))
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: technical demand %q is not Low/Moderate/High", ref, cellAt(row, cols.technicalDemand)))
			}
			balanceDemand, ok := parseDemand(cellAt(row, cols.balanceDemand))
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: balance demand %q is not Low/Moderate/High", ref, cellAt(row, cols.balanceDemand)))
			}

			approved := strings.EqualFold(strings.TrimSpace(cellAt(row, cols.reviewedByLina)), "yes")
			status := strengthlibrary.ReviewPending
			if approved {
				status = strengthlibrary.ReviewApproved
			}

			var flag, note string
			if cols.flag >= 0 {
				flag = cellAt(row, cols.flag)
			}
			if cols.note >= 0 {
				note = cellAt(row, cols.note)
			}

			exercises = append(exercises, strengthlibrary.Exercise{
				ID:              id,
				ActivityType:    "strength",
				Name:            name,
				Zone:            z,
				MovementGroup:   strengthlibrary.MovementGroup(group),
				ReviewStatus:    status,
				Equipment:       tags,
				TechnicalDemand: technicalDemand,
				BalanceDemand:   balanceDemand,
				DemandReviewed:  approved,
				Flag:            flag,
				Note:            note,
			})
		}
	}

	if len(errs) > 0 {
		return importResult{errs: errs}
	}

	sortExercises(exercises)
	sortNeedsReview(needsReview)
	return importResult{exercises: exercises, needsReview: needsReview}
}

// sortExercises/sortNeedsReview give the generated JSON a deterministic
// order (sorted by ID / by zone+group+name) so re-running the importer
// against unchanged source data produces a byte-identical file — real
// git diffs only ever show real sheet changes.
func sortExercises(exercises []strengthlibrary.Exercise) {
	sort.Slice(exercises, func(i, j int) bool { return exercises[i].ID < exercises[j].ID })
}

func sortNeedsReview(entries []needsReviewEntry) {
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Zone != b.Zone {
			return a.Zone < b.Zone
		}
		if a.MovementGroup != b.MovementGroup {
			return a.MovementGroup < b.MovementGroup
		}
		return a.Name < b.Name
	})
}
