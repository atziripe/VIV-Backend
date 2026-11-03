package strengthlibrary

import "strings"

// ExerciseID builds the stable slug ID for a (Zone, Name) pair — the
// single source of truth for this rule, so the importer and anything
// that ever needs to recompute/verify an ID (tests, a future migration)
// never drift into two different slugification implementations.
//
// Deliberately Zone+Name only, NOT Zone+MovementGroup+Name — this means
// the same exercise Name appearing twice in the same Zone under two
// different MovementGroups collides on ID. That's intentional: an ID is
// meant to identify "this exercise, in this zone," and a reviewer
// workbook that lists the same exercise name twice in one zone (even
// under different groups) almost certainly has a data problem, which
// the importer surfaces as a clear ID-collision error rather than
// silently producing two distinct IDs for what a consumer would read as
// one exercise.
func ExerciseID(zone Zone, name string) string {
	return string(zone) + "_" + slugify(name)
}

// slugify lowercases name and replaces every run of non [a-z0-9]
// characters with a single underscore, trimming leading/trailing
// underscores — "Goblet Squat" -> "goblet_squat".
func slugify(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastWasUnderscore := true // true so a leading separator never emits a leading "_"
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastWasUnderscore = false
		default:
			if !lastWasUnderscore {
				b.WriteByte('_')
				lastWasUnderscore = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "_")
}
