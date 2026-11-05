package main

import (
	"fmt"
	"strings"
)

// columnIndex says which column each recognized header occupies in one
// sheet. Optional columns (doesntBelong/comment/flag/note) are -1 when
// the sheet's layout doesn't carry them — both documented layouts
// ("VIV_Strength_Movement_Groups_Lina.xlsx" with header row 5, and
// "VIV_Strength_Library_v1.xlsx" with header row 1 and an added
// Flag/Note pair) are handled by the same struct; only the header row's
// actual contents decide which columns exist, never the file name or a
// hardcoded row number.
type columnIndex struct {
	movementGroup   int
	exercise        int
	equipment       int
	technicalDemand int
	balanceDemand   int
	reviewedByLina  int
	doesntBelong    int // -1 if this layout has no such column
	comment         int // -1 if this layout has no such column
	flag            int // -1 if this layout has no such column
	note            int // -1 if this layout has no such column
}

// requiredHeaders are the headers every supported layout must carry.
// "Movement group" and "Exercise" are also what identifies a row as the
// REAL header (as opposed to the unrelated review table some tabs keep
// in columns A-D on the left) — see findHeaderRow.
var requiredHeaders = []string{
	"movement group", "exercise", "equipment required",
	"technical demand", "balance demand", "reviewed by lina",
}

func normalizeHeader(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// findHeaderRow scans every row of a sheet looking for the one row that
// is genuinely the data table's header: a row containing a cell that
// normalizes to "movement group" AND a cell that normalizes to
// "exercise". That two-cell signature is deliberately how an old
// left-side review table (which doesn't carry those headers) is told
// apart from the real table, regardless of which column the real table
// starts in or which row it's on.
//
// Returns the 0-indexed row number and the resolved columnIndex, or an
// error naming the tab if no row in it carries that signature, or if a
// row has the signature but is missing one of requiredHeaders (neither
// documented layout, so this sheet can't be imported as-is).
func findHeaderRow(sh sheet) (int, columnIndex, error) {
	for rowIdx, row := range sh.rows {
		// First pass, unrestricted: just confirm this row carries both
		// signature headers somewhere. Real workbooks (not just the
		// hypothetical "old table in cols A-D" from the task
		// description) repeat "Exercise" as a header in BOTH the old
		// left-side table and the real one — e.g. "Exercise | How it's
		// prescribed now | Your call | Comment (optional) | | Movement
		// group | Exercise | Equipment required | ...". So this pass
		// only answers "is this a header row at all," never which
		// column anything lives in.
		firstOccurrence := make(map[string]int, len(row))
		for col, cell := range row {
			h := normalizeHeader(cell)
			if h == "" {
				continue
			}
			if _, exists := firstOccurrence[h]; !exists {
				firstOccurrence[h] = col
			}
		}

		mgCol, hasGroup := firstOccurrence["movement group"]
		_, hasExercise := firstOccurrence["exercise"]
		if !hasGroup || !hasExercise {
			continue
		}

		// Second pass, restricted to columns >= mgCol: "Movement
		// group" never appears in the old table (per the task's own
		// description of it), so its column anchors where the REAL
		// table starts. Resolving every other header from that point
		// on — instead of globally-first-occurrence — is what
		// correctly skips the old table's own "Exercise" column
		// sitting to its left.
		byHeader := make(map[string]int, len(row)-mgCol)
		for col := mgCol; col < len(row); col++ {
			h := normalizeHeader(row[col])
			if h == "" {
				continue
			}
			if _, exists := byHeader[h]; !exists {
				byHeader[h] = col
			}
		}

		var missing []string
		for _, h := range requiredHeaders {
			if _, ok := byHeader[h]; !ok {
				missing = append(missing, h)
			}
		}
		if len(missing) > 0 {
			return 0, columnIndex{}, fmt.Errorf(
				"tab %q: found a header row (row %d) but it's missing required column(s) %v at or after "+
					"the \"Movement group\" column — this doesn't match either documented layout",
				sh.tabName, rowIdx+1, missing,
			)
		}

		cols := columnIndex{
			movementGroup:   byHeader["movement group"],
			exercise:        byHeader["exercise"],
			equipment:       byHeader["equipment required"],
			technicalDemand: byHeader["technical demand"],
			balanceDemand:   byHeader["balance demand"],
			reviewedByLina:  byHeader["reviewed by lina"],
			doesntBelong:    -1,
			comment:         -1,
			flag:            -1,
			note:            -1,
		}
		if c, ok := byHeader["doesn't belong here?"]; ok {
			cols.doesntBelong = c
		}
		if c, ok := byHeader["comment"]; ok {
			cols.comment = c
		}
		if c, ok := byHeader["flag"]; ok {
			cols.flag = c
		}
		if c, ok := byHeader["note"]; ok {
			cols.note = c
		}
		return rowIdx, cols, nil
	}

	return 0, columnIndex{}, fmt.Errorf(
		"tab %q: no row has both a \"Movement group\" and an \"Exercise\" header cell — "+
			"neither documented layout found", sh.tabName,
	)
}
