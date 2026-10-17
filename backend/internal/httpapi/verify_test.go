package httpapi

import (
	"net/url"
	"strings"
	"testing"
)

func TestEmailVerification(t *testing.T) {
	h := newHarness(t)
	m := &captureMailer{}
	h.api.Mailer = m
	h.api.PublicBaseURL = "https://api.ww.test/"
	h.api.authLimiter.limit = 100
	tok := h.register("verify@example.com", "V")

	if me := h.do("GET", "/api/v1/me", tok, nil).json(t); me["email_verified_at"] != nil {
		t.Fatalf("new user verified: %v", me)
	}
	link := m.link("verify@example.com")
	if !strings.HasPrefix(link, "https://api.ww.test/api/v1/auth/email/verify?token=") {
		t.Fatalf("link %q", link)
	}
	// Resending invalidates the first link.
	if r := h.do("POST", "/api/v1/auth/email/resend", tok, nil); r.Status != 202 {
		t.Fatalf("resend %d", r.Status)
	}
	second := m.link("verify@example.com")
	if second == link {
		t.Fatal("resend reused the link")
	}
	path := func(l string) string { u, _ := url.Parse(l); return u.RequestURI() }
	if r := h.do("GET", path(link), "", nil); r.Status != 400 || !strings.Contains(string(r.Body), "Bağlantı geçersiz") {
		t.Fatalf("old link %d %s", r.Status, r.Body)
	}
	r := h.do("GET", path(second), "", nil)
	if r.Status != 200 || !strings.Contains(string(r.Body), "doğrulandı") || !strings.Contains(r.Header.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("verify %d %s", r.Status, r.Body)
	}
	if me := h.do("GET", "/api/v1/me", tok, nil).json(t); me["email_verified_at"] == nil {
		t.Fatalf("not verified: %v", me)
	}
	// Single use, and resend is a no-op once verified.
	if r := h.do("GET", path(second), "", nil); r.Status != 400 {
		t.Fatalf("reuse %d", r.Status)
	}
	if got := h.do("POST", "/api/v1/auth/email/resend", tok, nil).json(t); got["status"] != "already_verified" {
		t.Fatalf("resend after verify %v", got)
	}

	// JSON variant.
	tok2 := h.register("verify2@example.com", "W")
	_ = tok2
	l2, _ := url.Parse(m.link("verify2@example.com"))
	if r := h.do("POST", "/api/v1/auth/email/verify", "", map[string]string{"token": "nope"}); r.Status != 400 {
		t.Fatalf("bad token %d", r.Status)
	}
	if got := h.do("POST", "/api/v1/auth/email/verify", "", map[string]string{"token": l2.Query().Get("token")}).json(t); got["status"] != "verified" {
		t.Fatalf("json verify %v", got)
	}
	if r := h.do("POST", "/api/v1/auth/email/resend", "", nil); r.Status != 401 {
		t.Fatalf("resend without auth %d", r.Status)
	}
}
