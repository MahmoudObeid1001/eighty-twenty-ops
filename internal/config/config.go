package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL      string
	Port             string
	SessionSecret    string
	AdminEmail       string
	AdminPassword    string
	ModeratorEmail   string
	ModeratorPassword string
	Debug            bool
}

func Load() *Config {
	return &Config{
		DatabaseURL:      getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable"),
		Port:             getEnv("PORT", "3000"),
		SessionSecret:    getEnv("SESSION_SECRET", "change-this-to-a-random-secret-in-production"),
		AdminEmail:       getEnv("ADMIN_EMAIL", "admin@eightytwenty.test"),
		AdminPassword:    getEnv("ADMIN_PASSWORD", "admin123"),
		ModeratorEmail:   getEnv("MODERATOR_EMAIL", "moderator@eightytwenty.test"),
		ModeratorPassword: getEnv("MODERATOR_PASSWORD", "moderator123"),
		Debug:            getEnvBool("DEBUG", false),
	}
}

// Debugf logs a formatted message only when DEBUG is enabled
func (c *Config) Debugf(format string, v ...interface{}) {
	if c.Debug {
		log.Printf("[DEBUG] "+format, v...)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
