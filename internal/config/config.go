package config

import (
	"os"
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
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPPort:                getEnv("HTTP_PORT", "8080"),
		FirebaseProjectID:       getEnv("FIREBASE_PROJECT_ID", ""),
		FirebaseCredentialsFile: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		OpenAIAPIKey:            os.Getenv("OPENAI_API_KEY"),
		FirebaseProjectNumber:   getEnv("FIREBASE_PROJECT_NUMBER", ""),
		AppCheckEnforce:         getEnv("APP_CHECK_ENFORCE", "false") == "true",
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
