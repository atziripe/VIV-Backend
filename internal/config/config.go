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
}

func Load() (*Config, error) {
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
