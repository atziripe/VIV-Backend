package strengthlibrary

// demandOrdinal maps a DemandLevel to an ascending-sort rank. An unset
// (empty) level sorts as Low — current real content never leaves this
// blank on an approved exercise (verified against the real catalog),
// but a future gap shouldn't crash ordering; treating it as the
// easiest rating is the least-surprising default for a value that's
// explicitly "unreviewed, used for ordering only" anyway.
func demandOrdinal(d DemandLevel) int {
	switch d {
	case DemandModerate:
		return 1
	case DemandHigh:
		return 2
	default: // "" or DemandLow
		return 0
	}
}

// BuildSlotPool builds one slot's candidate pool from approvedByGroup
// (the zone's approved exercises, as returned by Library.ApprovedExercises,
// already filtered to approved-only and in each group's own file order)
// per VIV_Strength_Block_Config.json's ordering_rule: "pool ordered by
// (technical, balance) ratings ascending, ties interleaved across
// source groups; ratings unreviewed, used for ordering only."
//
// Concretely: every exercise across all of slot.MovementGroups is
// bucketed by its (TechnicalDemand, BalanceDemand) ordinal pair;
// buckets are emitted in ascending order. WITHIN one bucket, exercises
// from different source groups are interleaved round-robin (group A's
// first tied exercise, group B's first tied exercise, ..., then group
// A's second, ...) in slot.MovementGroups' own listed order — never
// grouping all of one source group's ties before another's, which is
// exactly what "ties interleaved across source groups" rules out.
// Within one group's own share of a bucket, its original (library
// file) order is preserved.
func BuildSlotPool(slot Slot, approvedByGroup map[MovementGroup][]Exercise) []Exercise {
	byBucket := make(map[bucketKey][]ranked)
	var bucketOrder []bucketKey
	seenBucket := make(map[bucketKey]bool)

	for groupIdx, group := range slot.MovementGroups {
		for _, ex := range approvedByGroup[group] {
			key := bucketKey{demandOrdinal(ex.TechnicalDemand), demandOrdinal(ex.BalanceDemand)}
			byBucket[key] = append(byBucket[key], ranked{groupIdx, ex})
			if !seenBucket[key] {
				seenBucket[key] = true
				bucketOrder = append(bucketOrder, key)
			}
		}
	}

	// Ascending (technical, balance) — insertion sort is plenty for the
	// handful of distinct buckets a slot ever produces, and keeps this
	// dependency-free; swap for sort.Slice if that ever stops being true.
	for i := 1; i < len(bucketOrder); i++ {
		for j := i; j > 0; j-- {
			a, b := bucketOrder[j-1], bucketOrder[j]
			if a.technical < b.technical || (a.technical == b.technical && a.balance <= b.balance) {
				break
			}
			bucketOrder[j-1], bucketOrder[j] = bucketOrder[j], bucketOrder[j-1]
		}
	}

	var pool []Exercise
	for _, key := range bucketOrder {
		pool = append(pool, interleaveByGroup(byBucket[key])...)
	}
	return pool
}

// interleaveByGroup round-robins ranked items across their groupIdx
// values, in ascending groupIdx order (i.e. slot.MovementGroups' own
// listed order), preserving each group's relative order among its own
// items.
func interleaveByGroup(items []ranked) []Exercise {
	perGroup := make(map[int][]Exercise)
	var groupOrder []int
	seenGroup := make(map[int]bool)
	for _, it := range items {
		perGroup[it.groupIdx] = append(perGroup[it.groupIdx], it.exercise)
		if !seenGroup[it.groupIdx] {
			seenGroup[it.groupIdx] = true
			groupOrder = append(groupOrder, it.groupIdx)
		}
	}
	for i := 1; i < len(groupOrder); i++ {
		for j := i; j > 0 && groupOrder[j-1] > groupOrder[j]; j-- {
			groupOrder[j-1], groupOrder[j] = groupOrder[j], groupOrder[j-1]
		}
	}

	var out []Exercise
	for {
		emitted := false
		for _, gi := range groupOrder {
			if len(perGroup[gi]) > 0 {
				out = append(out, perGroup[gi][0])
				perGroup[gi] = perGroup[gi][1:]
				emitted = true
			}
		}
		if !emitted {
			break
		}
	}
	return out
}

type bucketKey struct{ technical, balance int }

type ranked struct {
	groupIdx int
	exercise Exercise
}
