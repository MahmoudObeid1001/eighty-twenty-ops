package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL              string
	Port                     string
	SessionSecret            string
	AdminEmail               string
	AdminPassword            string
	ModeratorEmail           string
	ModeratorPassword        string
	MentorHeadEmail          string
	MentorHeadPassword       string
	MentorEmail              string
	MentorPassword           string
	CommunityOfficerEmail    string
	CommunityOfficerPassword string
	HREmail                  string
	HRPassword               string
	StudentSuccessEmail      string
	StudentSuccessPassword   string
	Debug                    bool
}

func Load() *Config {
	return &Config{
		DatabaseURL:              getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable"),
		Port:                     getEnv("PORT", "3000"),
		SessionSecret:            getEnv("SESSION_SECRET", "change-this-to-a-random-secret-in-production"),
		AdminEmail:               getEnv("ADMIN_EMAIL", "admin@eightytwenty.test"),
		AdminPassword:            getEnv("ADMIN_PASSWORD", "admin123"),
		ModeratorEmail:           getEnv("MODERATOR_EMAIL", "moderator@eightytwenty.test"),
		ModeratorPassword:        getEnv("MODERATOR_PASSWORD", "moderator123"),
		MentorHeadEmail:          getEnv("MENTOR_HEAD_EMAIL", "mentor_head@eightytwenty.test"),
		MentorHeadPassword:       getEnv("MENTOR_HEAD_PASSWORD", "mentor_head123"),
		MentorEmail:              getEnv("MENTOR_EMAIL", "mentor@eightytwenty.test"),
		MentorPassword:           getEnv("MENTOR_PASSWORD", "mentor123"),
		CommunityOfficerEmail:    getEnv("COMMUNITY_OFFICER_EMAIL", "community_officer@eightytwenty.test"),
		CommunityOfficerPassword: getEnv("COMMUNITY_OFFICER_PASSWORD", "community_officer123"),
		HREmail:                  getEnv("HR_EMAIL", "hr@eightytwenty.test"),
		HRPassword:               getEnv("HR_PASSWORD", "hr123"),
		StudentSuccessEmail:      getEnv("STUDENT_SUCCESS_EMAIL", "student_success@eightytwenty.test"),
		StudentSuccessPassword:   getEnv("STUDENT_SUCCESS_PASSWORD", "student_success123"),
		Debug:                    getEnvBool("DEBUG", false),
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
