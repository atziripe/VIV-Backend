package strengthlibrary

// BuildSlotPool builds one slot's candidate pool from approvedByGroup
// (the zone's approved exercises, as returned by Library.ApprovedExercises,
// already filtered to approved-only) per VIV_Strength_Block_Config.json's
// current ordering_rule: each slot carries its own explicit PoolOrder —
// a list of exercise names, in the exact order the config author wants
// them tried. This function never re-sorts that order by any rating or
// heuristic of its own; it only resolves each name to the matching
// approved Exercise (across all of slot.MovementGroups) and skips any
// name that isn't currently approved — pending, rejected, or simply
// absent from the catalog — exactly per the config's own ordering_rule
// ("skip any exercise that is not approved in the catalog"). A name
// that never resolves is silently omitted, never an error: the config
// is allowed to list exercises that aren't approved yet, anticipating
// a future review.
func BuildSlotPool(slot Slot, approvedByGroup map[MovementGroup][]Exercise) []Exercise {
	byName := make(map[string]Exercise)
	for _, group := range slot.MovementGroups {
		for _, ex := range approvedByGroup[group] {
			byName[ex.Name] = ex
		}
	}

	pool := make([]Exercise, 0, len(slot.PoolOrder))
	for _, name := range slot.PoolOrder {
		if ex, ok := byName[name]; ok {
			pool = append(pool, ex)
		}
	}
	return pool
}
