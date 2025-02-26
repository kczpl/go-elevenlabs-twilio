package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port              string
	ElevenLabsAPIKey  string
	ElevenLabsAgentID string
	TwilioAccountSID  string
	TwilioAuthToken   string
	TwilioPhoneNumber string
	ApiAuthToken      string
	ApiURL            string
	Environment       string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:              getEnv("PORT", "8000"),
		ElevenLabsAPIKey:  getEnv("ELEVENLABS_API_KEY", ""),
		ElevenLabsAgentID: getEnv("ELEVENLABS_AGENT_ID", ""),
		TwilioAccountSID:  getEnv("TWILIO_ACCOUNT_SID", ""),
		TwilioAuthToken:   getEnv("TWILIO_AUTH_TOKEN", ""),
		TwilioPhoneNumber: getEnv("TWILIO_PHONE_NUMBER", ""),
		Environment:       getEnv("ENV", "development"),
	}

	// Validate required environment variables
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
func (c *Config) validate() error {
	required := map[string]string{
		"ELEVENLABS_API_KEY":  c.ElevenLabsAPIKey,
		"ELEVENLABS_AGENT_ID": c.ElevenLabsAgentID,
		"TWILIO_ACCOUNT_SID":  c.TwilioAccountSID,
		"TWILIO_AUTH_TOKEN":   c.TwilioAuthToken,
		"TWILIO_PHONE_NUMBER": c.TwilioPhoneNumber,
	}

	for name, value := range required {
		if value == "" {
			return fmt.Errorf("missing required environment variable: %s", name)
		}
	}

	return nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
