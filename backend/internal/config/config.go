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
	RefreshTTL      time.Duration
	TrustProxy      bool
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
	PublicBaseURL   string // used in e-mail links
	SMTPHost        string // empty → mails are only logged
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string
	S3Endpoint      string // with S3Bucket set, files go to S3/MinIO instead of StorageDir
	S3Bucket        string
	S3Region        string
	S3AccessKey     string
	S3SecretKey     string
	S3PathStyle     bool
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
		TokenTTL:      dur(getenv("TOKEN_TTL"), time.Hour),
		RefreshTTL:    dur(getenv("REFRESH_TTL"), 30*24*time.Hour),
		TrustProxy:    strings.EqualFold(strings.TrimSpace(getenv("TRUST_PROXY")), "true"),
		StorageDir:    def(getenv("STORAGE_DIR"), "./uploads"),
		MaxUploadMB:   int64(num(getenv("MAX_UPLOAD_MB"), 10)),
		GeminiAPIKey:  strings.TrimSpace(getenv("GEMINI_API_KEY")),
		GeminiModel:   def(getenv("GEMINI_MODEL"), "gemini-3.5-flash"),
		GeminiBaseURL: def(getenv("GEMINI_BASE_URL"), "https://generativelanguage.googleapis.com/v1beta"),
		GeminiTimeout: dur(getenv("GEMINI_TIMEOUT"), 25*time.Second),
		CORSOrigins:   def(getenv("CORS_ORIGINS"), "*"),
		NotifyEvery:   dur(getenv("NOTIFY_INTERVAL"), 15*time.Minute),
		PublicBaseURL: strings.TrimRight(def(getenv("PUBLIC_BASE_URL"), "http://localhost:8080"), "/"),
		SMTPHost:      strings.TrimSpace(getenv("SMTP_HOST")),
		SMTPPort:      num(getenv("SMTP_PORT"), 587),
		SMTPUsername:  strings.TrimSpace(getenv("SMTP_USERNAME")),
		SMTPPassword:  getenv("SMTP_PASSWORD"),
		SMTPFrom:      strings.TrimSpace(getenv("SMTP_FROM")),
		S3Endpoint:    strings.TrimSpace(getenv("S3_ENDPOINT")),
		S3Bucket:      strings.TrimSpace(getenv("S3_BUCKET")),
		S3Region:      def(getenv("S3_REGION"), "us-east-1"),
		S3AccessKey:   strings.TrimSpace(getenv("S3_ACCESS_KEY")),
		S3SecretKey:   getenv("S3_SECRET_KEY"),
		S3PathStyle:   !strings.EqualFold(strings.TrimSpace(getenv("S3_PATH_STYLE")), "false"),
	}
	if c.S3Bucket != "" && (c.S3Endpoint == "" || c.S3AccessKey == "" || c.S3SecretKey == "") {
		return c, errors.New("S3_ENDPOINT, S3_ACCESS_KEY and S3_SECRET_KEY are required when S3_BUCKET is set")
	}
	if c.SMTPHost != "" && c.SMTPFrom == "" {
		return c, errors.New("SMTP_FROM is required when SMTP_HOST is set")
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
