package config

import (
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDefaults(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8080" || c.GeminiModel == "" || c.MaxUploadMB != 10 || c.TokenTTL != 7*24*time.Hour || c.JWTSecret == "" {
		t.Fatalf("%+v", c)
	}
	if c.GeminiAPIKey != "" {
		t.Fatal("key must default to empty")
	}
}

func TestOverridesAndProduction(t *testing.T) {
	c, err := Load(env(map[string]string{"GEMINI_API_KEY": "  k  ", "ADMIN_EMAILS": "A@x.io, b@y.io,", "MAX_UPLOAD_MB": "5", "TOKEN_TTL": "1h"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.GeminiAPIKey != "k" || len(c.AdminEmails) != 2 || c.AdminEmails[0] != "a@x.io" || c.MaxUploadMB != 5 || c.TokenTTL != time.Hour {
		t.Fatalf("%+v", c)
	}
	if _, err := Load(env(map[string]string{"APP_ENV": "production"})); err == nil {
		t.Fatal("production without JWT_SECRET must fail")
	}
	if _, err := Load(env(map[string]string{"APP_ENV": "production", "JWT_SECRET": "short"})); err == nil {
		t.Fatal("short secret must fail in production")
	}
}
