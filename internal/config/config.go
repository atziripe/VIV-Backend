package config

import (
	"os"
)

type Config struct {
	HTTPPort                string
	FirebaseProjectID       string
	FirebaseCredentialsFile string
	OpenAIAPIKey            string
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPPort:                getEnv("HTTP_PORT", "8080"),
		FirebaseProjectID:       getEnv("FIREBASE_PROJECT_ID", ""),
		FirebaseCredentialsFile: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		OpenAIAPIKey:            os.Getenv("OPENAI_API_KEY"),
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
