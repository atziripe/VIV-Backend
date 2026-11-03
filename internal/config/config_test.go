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
