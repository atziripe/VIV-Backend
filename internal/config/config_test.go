package config

import "testing"

func TestLoad_IncludePendingExercises_ForcedOffOutsideDevStaging(t *testing.T) {
	cases := []struct {
		appEnv string
		want   bool
	}{
		{"development", true},
		{"staging", true},
		{"production", false},
		{"", false},
		{"prod", false},
	}

	for _, tc := range cases {
		t.Setenv("APP_ENV", tc.appEnv)
		t.Setenv("INCLUDE_PENDING_EXERCISES", "true")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() with APP_ENV=%q: %v", tc.appEnv, err)
		}
		if cfg.IncludePendingExercises != tc.want {
			t.Errorf("APP_ENV=%q: IncludePendingExercises = %v, want %v", tc.appEnv, cfg.IncludePendingExercises, tc.want)
		}
	}
}

func TestLoad_IncludePendingExercises_DefaultFalse(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("INCLUDE_PENDING_EXERCISES", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.IncludePendingExercises {
		t.Error("expected IncludePendingExercises to default to false when the env var is unset")
	}
}

func TestLoad_UseRealStrengthSessions_DefaultsByEnvironment(t *testing.T) {
	cases := []struct {
		appEnv string
		want   bool
	}{
		{"development", true},
		{"staging", false},
		{"production", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Setenv("APP_ENV", tc.appEnv)
		t.Setenv("USE_REAL_STRENGTH_SESSIONS", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() with APP_ENV=%q: %v", tc.appEnv, err)
		}
		if cfg.UseRealStrengthSessions != tc.want {
			t.Errorf("APP_ENV=%q: UseRealStrengthSessions = %v, want %v", tc.appEnv, cfg.UseRealStrengthSessions, tc.want)
		}
	}
}

func TestLoad_UseRealStrengthSessions_ExplicitEnvVarAlwaysWins(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("USE_REAL_STRENGTH_SESSIONS", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if !cfg.UseRealStrengthSessions {
		t.Error("expected an explicit USE_REAL_STRENGTH_SESSIONS=true to win even in production — this is a plain feature flag, not a safety gate like IncludePendingExercises")
	}

	t.Setenv("APP_ENV", "development")
	t.Setenv("USE_REAL_STRENGTH_SESSIONS", "false")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.UseRealStrengthSessions {
		t.Error("expected an explicit USE_REAL_STRENGTH_SESSIONS=false to win even in development")
	}
}

func TestLoad_StrengthSessionsAllowlist_UnsetIsNil(t *testing.T) {
	t.Setenv("STRENGTH_SESSIONS_ALLOWLIST", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.StrengthSessionsAllowlist != nil {
		t.Errorf("StrengthSessionsAllowlist = %v, want nil when unset", cfg.StrengthSessionsAllowlist)
	}
}

func TestLoad_StrengthSessionsAllowlist_SplitsTrimsAndDropsEmpties(t *testing.T) {
	t.Setenv("STRENGTH_SESSIONS_ALLOWLIST", " u1, u2 ,,u3")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	want := []string{"u1", "u2", "u3"}
	if len(cfg.StrengthSessionsAllowlist) != len(want) {
		t.Fatalf("StrengthSessionsAllowlist = %v, want %v", cfg.StrengthSessionsAllowlist, want)
	}
	for i := range want {
		if cfg.StrengthSessionsAllowlist[i] != want[i] {
			t.Errorf("StrengthSessionsAllowlist[%d] = %q, want %q", i, cfg.StrengthSessionsAllowlist[i], want[i])
		}
	}
}
