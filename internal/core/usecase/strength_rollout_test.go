package usecase_test

import (
	"testing"

	"viv/internal/core/usecase"
)

// ============================================================================
// StrengthSessionsRollout — three modes:
//  1. master off -> nobody, regardless of allowlist
//  2. master on, empty allowlist -> everyone
//  3. master on, non-empty allowlist -> only listed users
// ============================================================================

func TestStrengthSessionsRollout_MasterOffNobodyEnabled(t *testing.T) {
	r := usecase.NewStrengthSessionsRollout(false, []string{"u1", "u2"})
	for _, userID := range []string{"u1", "u2", "someone_else"} {
		if r.Enabled(userID) {
			t.Errorf("Enabled(%q) = true, want false — the master switch is off", userID)
		}
	}
}

func TestStrengthSessionsRollout_MasterOnEmptyAllowlistEveryoneEnabled(t *testing.T) {
	r := usecase.NewStrengthSessionsRollout(true, nil)
	for _, userID := range []string{"u1", "u2", "anyone_at_all"} {
		if !r.Enabled(userID) {
			t.Errorf("Enabled(%q) = false, want true — master on with an empty allowlist means everyone", userID)
		}
	}
}

func TestStrengthSessionsRollout_MasterOnNonEmptyAllowlistOnlyListedUsers(t *testing.T) {
	r := usecase.NewStrengthSessionsRollout(true, []string{"u1", "u2"})
	if !r.Enabled("u1") {
		t.Error("Enabled(u1) = false, want true — u1 is on the allowlist")
	}
	if !r.Enabled("u2") {
		t.Error("Enabled(u2) = false, want true — u2 is on the allowlist")
	}
	if r.Enabled("u3") {
		t.Error("Enabled(u3) = true, want false — u3 is not on the allowlist")
	}
}

func TestStrengthSessionsRollout_MasterOnNonEmptyAllowlistButMasterOffStillNobody(t *testing.T) {
	// The master switch always wins off, even with a non-empty
	// allowlist — mode 1 takes priority over mode 3.
	r := usecase.NewStrengthSessionsRollout(false, []string{"u1"})
	if r.Enabled("u1") {
		t.Error("Enabled(u1) = true, want false — the master switch is off, the allowlist is irrelevant")
	}
}

func TestStrengthSessionsRollout_Mode_NamesEachCase(t *testing.T) {
	if got := usecase.NewStrengthSessionsRollout(false, nil).Mode(); got != "disabled" {
		t.Errorf("Mode() = %q, want %q", got, "disabled")
	}
	if got := usecase.NewStrengthSessionsRollout(true, nil).Mode(); got != "enabled for everyone" {
		t.Errorf("Mode() = %q, want %q", got, "enabled for everyone")
	}
	got := usecase.NewStrengthSessionsRollout(true, []string{"u1"}).Mode()
	if got != "allowlist only (u1)" {
		t.Errorf("Mode() = %q, want %q", got, "allowlist only (u1)")
	}
}
