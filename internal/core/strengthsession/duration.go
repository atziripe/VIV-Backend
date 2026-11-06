package strengthsession

import "viv/internal/core/strengthlibrary"

// dropForShort implements DurationTier=Short: try each priority in
// rules.DropOrder in turn (Optional, then Accessory); within a
// priority, drop from the LAST slot of that priority back toward the
// first ("from the last one up"); never drop a priority in
// rules.NeverDrop; stop dropping the moment doing so would take the
// total below rules.MinExercises. Returns the entries that survive, in
// their original relative order, plus the ones that got dropped (for
// the caller's TierChange detail text).
func dropForShort(entries []slotEntry, rules strengthlibrary.ShortRules) (kept []slotEntry, dropped []slotEntry) {
	neverDrop := make(map[strengthlibrary.SlotPriority]bool, len(rules.NeverDrop))
	for _, p := range rules.NeverDrop {
		neverDrop[p] = true
	}

	kept = make([]slotEntry, len(entries))
	copy(kept, entries)

	for _, priority := range rules.DropOrder {
		if neverDrop[priority] {
			continue // config validation should prevent this overlap, but never drop a protected priority regardless
		}
		// Iterate backward so removing index i never disturbs the
		// indices still left to visit (0..i-1).
		for i := len(kept) - 1; i >= 0 && len(kept) > rules.MinExercises; i-- {
			if kept[i].slot.Priority != priority {
				continue
			}
			dropped = append(dropped, kept[i])
			kept = append(kept[:i], kept[i+1:]...)
		}
	}
	return kept, dropped
}
