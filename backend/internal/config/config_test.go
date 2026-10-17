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
	if c.HTTPAddr != ":8080" || c.GeminiModel == "" || c.MaxUploadMB != 10 || c.TokenTTL != time.Hour || c.RefreshTTL != 30*24*time.Hour || c.TrustProxy || c.JWTSecret == "" {
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

func TestGeminiFallbackModels(t *testing.T) {
	c, _ := Load(func(string) string { return "" })
	if len(c.GeminiFallbacks) != 1 || c.GeminiFallbacks[0] != "gemini-2.5-flash" {
		t.Fatalf("default fallbacks %v", c.GeminiFallbacks)
	}
	env := map[string]string{"GEMINI_MODEL": "m1", "GEMINI_FALLBACK_MODELS": " m1, m2 ,,m3"}
	c, _ = Load(func(k string) string { return env[k] })
	if len(c.GeminiFallbacks) != 2 || c.GeminiFallbacks[0] != "m2" || c.GeminiFallbacks[1] != "m3" {
		t.Fatalf("fallbacks %v", c.GeminiFallbacks)
	}
}

func TestSMTPConfig(t *testing.T) {
	env := map[string]string{"SMTP_HOST": "smtp.example.com", "PUBLIC_BASE_URL": "https://api.example.com/"}
	if _, err := Load(func(k string) string { return env[k] }); err == nil {
		t.Fatal("SMTP_HOST without SMTP_FROM must fail")
	}
	env["SMTP_FROM"] = "Warranty Wallet <noreply@example.com>"
	c, err := Load(func(k string) string { return env[k] })
	if err != nil || c.SMTPPort != 587 || c.PublicBaseURL != "https://api.example.com" {
		t.Fatalf("%+v %v", c, err)
	}
	c, _ = Load(func(string) string { return "" })
	if c.SMTPHost != "" || c.PublicBaseURL != "http://localhost:8080" {
		t.Fatalf("defaults %+v", c)
	}
}
