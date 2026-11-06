package usecase

import "strings"

// StrengthSessionsRollout decides, per user, whether a Strength slot
// gets the real VIV-112 assembly path or the OLD (content.ExerciseLibrary
// placeholder) path — see SessionContentSelector.SelectContent.
//
// MasterEnabled is config's USE_REAL_STRENGTH_SESSIONS — the master
// switch. Allowlist, when non-empty, further restricts the new path to
// exactly those user IDs; when empty, the master switch alone decides
// for everyone. Three modes, in order:
//
//  1. MasterEnabled == false: nobody gets the new path, regardless of
//     Allowlist — the master switch always wins off.
//  2. MasterEnabled == true, Allowlist non-empty: only listed users get
//     the new path; everyone else keeps the old one.
//  3. MasterEnabled == true, Allowlist empty: everyone gets the new
//     path.
type StrengthSessionsRollout struct {
	MasterEnabled bool
	Allowlist     map[string]bool
}

// NewStrengthSessionsRollout builds a StrengthSessionsRollout from
// config's two raw values — masterEnabled (USE_REAL_STRENGTH_SESSIONS)
// and allowlistUserIDs (STRENGTH_SESSIONS_ALLOWLIST, already split on
// comma and trimmed by config.Load). A nil or empty allowlistUserIDs
// means mode 3 (everyone, once the master switch is on).
func NewStrengthSessionsRollout(masterEnabled bool, allowlistUserIDs []string) StrengthSessionsRollout {
	var allowlist map[string]bool
	if len(allowlistUserIDs) > 0 {
		allowlist = make(map[string]bool, len(allowlistUserIDs))
		for _, id := range allowlistUserIDs {
			allowlist[id] = true
		}
	}
	return StrengthSessionsRollout{MasterEnabled: masterEnabled, Allowlist: allowlist}
}

// Enabled reports whether userID gets the real Strength assembly path.
func (r StrengthSessionsRollout) Enabled(userID string) bool {
	if !r.MasterEnabled {
		return false
	}
	if len(r.Allowlist) == 0 {
		return true
	}
	return r.Allowlist[userID]
}

// Mode names the active rollout mode, for the one startup log line —
// never used for behavior, only for a human reading logs.
func (r StrengthSessionsRollout) Mode() string {
	switch {
	case !r.MasterEnabled:
		return "disabled"
	case len(r.Allowlist) == 0:
		return "enabled for everyone"
	default:
		return "allowlist only (" + strings.Join(sortedKeys(r.Allowlist), ", ") + ")"
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
