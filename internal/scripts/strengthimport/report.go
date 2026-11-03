package main

import (
	"fmt"
	"io"
	"sort"

	"viv/internal/core/strengthlibrary"
)

// thinGroupThreshold mirrors the task's "fewer than 4 approved
// exercises" — a group at or under this count is flagged as thin so
// the team can see where variety is missing.
const thinGroupThreshold = 4

type groupKey struct {
	zone  strengthlibrary.Zone
	group strengthlibrary.MovementGroup
}

// printReport writes the three summaries the importer owes after a
// successful run: counts per zone, counts per movement group, and
// which (zone, group) pairs are thin on APPROVED exercises — a pending
// row doesn't help anyone generate a session yet, so thinness is judged
// only on what's actually selectable today.
func printReport(w io.Writer, exercises []strengthlibrary.Exercise) {
	perZone := make(map[strengthlibrary.Zone]int)
	total := make(map[groupKey]int)
	approved := make(map[groupKey]int)
	var keys []groupKey
	seenKey := make(map[groupKey]bool)

	for _, e := range exercises {
		perZone[e.Zone]++
		k := groupKey{e.Zone, e.MovementGroup}
		total[k]++
		if e.ReviewStatus == strengthlibrary.ReviewApproved {
			approved[k]++
		}
		if !seenKey[k] {
			seenKey[k] = true
			keys = append(keys, k)
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].zone != keys[j].zone {
			return keys[i].zone < keys[j].zone
		}
		return keys[i].group < keys[j].group
	})

	fmt.Fprintln(w, "=== Exercises per zone ===")
	for _, z := range strengthlibrary.AllZones {
		fmt.Fprintf(w, "  %-6s %d\n", z, perZone[z])
	}

	fmt.Fprintln(w, "=== Exercises per movement group ===")
	for _, k := range keys {
		fmt.Fprintf(w, "  %-6s / %-20s total=%d approved=%d\n", k.zone, k.group, total[k], approved[k])
	}

	fmt.Fprintln(w, "=== Thin groups (fewer than 4 approved exercises) ===")
	thinCount := 0
	for _, k := range keys {
		if approved[k] < thinGroupThreshold {
			fmt.Fprintf(w, "  %-6s / %-20s only %d approved\n", k.zone, k.group, approved[k])
			thinCount++
		}
	}
	if thinCount == 0 {
		fmt.Fprintln(w, "  (none)")
	}
}
