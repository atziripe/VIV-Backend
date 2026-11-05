package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPPort                string
	FirebaseProjectID       string
	FirebaseCredentialsFile string
	OpenAIAPIKey            string

	// FirebaseProjectNumber is the numeric project number (not
	// FirebaseProjectID, the string id) — App Check tokens' iss/aud claims
	// are keyed by project number. Find it in Firebase console → Project
	// settings → General → "Project number". Leave unset to disable App
	// Check verification entirely (see http.AppCheckMiddleware).
	FirebaseProjectNumber string

	// AppCheckEnforce gates whether a missing/invalid App Check token
	// actually blocks a request (401) or is only logged. Defaults to
	// false (monitor mode) — flip APP_CHECK_ENFORCE=true only once the
	// mobile client's App Check SDK integration is shipped and real
	// traffic is carrying valid tokens (check the App Check console's
	// request metrics first); enforcing before that locks every user out.
	AppCheckEnforce bool

	// BrevoAPIKey/BrevoInactivityTemplateID configure the 2-year-inactivity
	// re-engagement email (see email.BrevoReEngagementEmailSender /
	// usecase.RunInactivitySweepUseCase). The template's copy is managed in
	// Brevo's own dashboard, not here — BrevoInactivityTemplateID just
	// selects which one to send. Leaving BrevoAPIKey unset means the
	// inactivity sweep will fail every send (and therefore never start the
	// 30-day notice clock) rather than silently skip it — see main.go's
	// wiring for why that's treated as a real gap, not a soft no-op.
	BrevoAPIKey               string
	BrevoInactivityTemplateID int

	// Apple* configure Sign in with Apple token revocation on account
	// deletion (App Store guideline 5.1.1v — see
	// apple.TokenRevoker/usecase.DeleteAccountUseCase). All four must be
	// set together or revocation is skipped entirely (logged once at
	// startup, see main.go) — a partial set is treated the same as none,
	// never a half-working revoker.
	//
	// AppleTeamID: Apple Developer "Team ID".
	// AppleKeyID: the Key ID of the "Sign in with Apple" private key.
	// AppleClientID: the Services ID (or App ID) used as Sign in with
	// Apple's client_id — not the Key ID.
	// ApplePrivateKey: the contents of the AuthKey_<KeyID>.p8 file. Most
	// deployment platforms can't carry real newlines in an env var value,
	// so a literal `\n` is accepted and converted to a real newline before
	// use (same convention as Firebase's credentials JSON elsewhere).
	AppleTeamID     string
	AppleKeyID      string
	AppleClientID   string
	ApplePrivateKey string

	// AppEnv is the deployment environment ("production", "staging",
	// "development", ...) — same raw value main.go's sentry.Init already
	// reads directly from APP_ENV for its own Environment option. Centralized
	// here too so IncludePendingExercises (below) has something to gate on
	// without every reader re-parsing the env var its own way.
	AppEnv string

	// IncludePendingExercises exposes exercises still pending Lina's
	// review through strengthlibrary.Library.ExercisesForZone's dev-only
	// escape hatch, instead of only the Approved set
	// (strengthlibrary.Library.ApprovedExercises always ignores this
	// flag completely — see its doc comment). Defaults to false, and
	// Load forces it back to false outside development/staging no
	// matter what INCLUDE_PENDING_EXERCISES says — a misconfigured
	// production env var can never turn this on.
	IncludePendingExercises bool

	// UseRealStrengthSessions switches Strength session assembly
	// (VIV-112, internal/core/strengthsession) on — the real VIV-108
	// pinned block hydrated against the real strengthlibrary catalog,
	// instead of the OLD path (content.ExerciseLibrary's placeholder
	// seed data). Every other activity type is unaffected either way.
	// Unlike IncludePendingExercises, this is a plain feature flag, not
	// a safety gate — USE_REAL_STRENGTH_SESSIONS, when explicitly set,
	// always wins; only the default (unset) differs by environment:
	// true in development, false everywhere else, until the team
	// decides it's ready for staging/production.
	UseRealStrengthSessions bool
}

func Load() (*Config, error) {
	appEnv := os.Getenv("APP_ENV")
	includePending := getEnv("INCLUDE_PENDING_EXERCISES", "false") == "true"
	if includePending && appEnv != "development" && appEnv != "staging" {
		// Fail-safe, not a validation error: a stray env var in
		// production should never surface pending, unreviewed Strength
		// exercises to real users.
		includePending = false
	}

	useRealStrengthSessions := appEnv == "development"
	if v := os.Getenv("USE_REAL_STRENGTH_SESSIONS"); v != "" {
		useRealStrengthSessions = v == "true"
	}

	cfg := &Config{
		HTTPPort:                getEnv("HTTP_PORT", "8080"),
		FirebaseProjectID:       getEnv("FIREBASE_PROJECT_ID", ""),
		FirebaseCredentialsFile: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		OpenAIAPIKey:            os.Getenv("OPENAI_API_KEY"),
		FirebaseProjectNumber:   getEnv("FIREBASE_PROJECT_NUMBER", ""),
		AppCheckEnforce:         getEnv("APP_CHECK_ENFORCE", "false") == "true",

		BrevoAPIKey:               os.Getenv("BREVO_API_KEY"),
		BrevoInactivityTemplateID: getEnvInt("BREVO_INACTIVITY_TEMPLATE_ID", 0),

		AppleTeamID:     os.Getenv("APPLE_TEAM_ID"),
		AppleKeyID:      os.Getenv("APPLE_KEY_ID"),
		AppleClientID:   os.Getenv("APPLE_CLIENT_ID"),
		ApplePrivateKey: strings.ReplaceAll(os.Getenv("APPLE_PRIVATE_KEY"), `\n`, "\n"),

		AppEnv:                  appEnv,
		IncludePendingExercises: includePending,
		UseRealStrengthSessions: useRealStrengthSessions,
	}
	// podrías validar aquí que ProjectID no esté vacío, etc.
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
