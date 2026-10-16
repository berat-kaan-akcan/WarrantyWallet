// Package config loads runtime configuration. Secrets come ONLY from environment variables.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the backend configuration.
type Config struct {
	HTTPAddr        string
	DatabaseURL     string // empty → in-memory store (dev/demo only)
	JWTSecret       string
	TokenTTL        time.Duration
	StorageDir      string
	MaxUploadMB     int64
	GeminiAPIKey    string
	GeminiModel     string
	GeminiFallbacks []string
	GeminiBaseURL   string
	GeminiTimeout   time.Duration
	CORSOrigins     string
	AdminEmails     []string
	NotifyEvery     time.Duration
}

// Load reads configuration from the environment (getenv is injectable for tests).
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	c := Config{
		HTTPAddr:      def(getenv("HTTP_ADDR"), ":8080"),
		DatabaseURL:   getenv("DATABASE_URL"),
		JWTSecret:     getenv("JWT_SECRET"),
		TokenTTL:      dur(getenv("TOKEN_TTL"), 7*24*time.Hour),
		StorageDir:    def(getenv("STORAGE_DIR"), "./uploads"),
		MaxUploadMB:   int64(num(getenv("MAX_UPLOAD_MB"), 10)),
		GeminiAPIKey:  strings.TrimSpace(getenv("GEMINI_API_KEY")),
		GeminiModel:   def(getenv("GEMINI_MODEL"), "gemini-3.5-flash"),
		GeminiBaseURL: def(getenv("GEMINI_BASE_URL"), "https://generativelanguage.googleapis.com/v1beta"),
		GeminiTimeout: dur(getenv("GEMINI_TIMEOUT"), 25*time.Second),
		CORSOrigins:   def(getenv("CORS_ORIGINS"), "*"),
		NotifyEvery:   dur(getenv("NOTIFY_INTERVAL"), 15*time.Minute),
	}
	for _, m := range strings.Split(def(getenv("GEMINI_FALLBACK_MODELS"), "gemini-2.5-flash"), ",") {
		if m = strings.TrimSpace(m); m != "" && m != c.GeminiModel {
			c.GeminiFallbacks = append(c.GeminiFallbacks, m)
		}
	}
	for _, e := range strings.Split(getenv("ADMIN_EMAILS"), ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			c.AdminEmails = append(c.AdminEmails, e)
		}
	}
	if c.JWTSecret == "" {
		if getenv("APP_ENV") == "production" {
			return c, errors.New("JWT_SECRET is required in production")
		}
		c.JWTSecret = "dev-insecure-secret-change-me"
	}
	if len(c.JWTSecret) < 16 && getenv("APP_ENV") == "production" {
		return c, errors.New("JWT_SECRET must be at least 16 characters")
	}
	return c, nil
}

func def(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return strings.TrimSpace(v)
}

func num(v string, d int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
		return n
	}
	return d
}

func dur(v string, d time.Duration) time.Duration {
	if x, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && x > 0 {
		return x
	}
	return d
}
