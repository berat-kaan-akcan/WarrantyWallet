package httpapi

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

type captureMailer struct {
	mu     sync.Mutex
	tokens map[string]string // email → last token
}

func (c *captureMailer) SendPasswordReset(_ context.Context, to, _ string, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokens == nil {
		c.tokens = map[string]string{}
	}
	c.tokens[to] = token
	return nil
}

func (c *captureMailer) last(email string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens[email]
}

func login(h *harness, email, password string) resp {
	h.t.Helper()
	return h.do("POST", "/api/v1/auth/login", "", map[string]string{"email": email, "password": password})
}

func TestRefreshTokenRotation(t *testing.T) {
	h := newHarness(t)
	h.register("rt@example.com", "RT")
	r := login(h, "rt@example.com", "supersecret")
	body := r.json(t)
	rt, _ := body["refresh_token"].(string)
	if rt == "" || body["refresh_expires_at"] == nil {
		t.Fatalf("no refresh token: %s", r.Body)
	}
	r2 := h.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": rt})
	if r2.Status != 200 {
		t.Fatalf("refresh %d %s", r2.Status, r2.Body)
	}
	b2 := r2.json(t)
	if b2["token"] == "" || b2["refresh_token"] == rt {
		t.Fatalf("refresh must rotate: %s", r2.Body)
	}
	if me := h.do("GET", "/api/v1/me", b2["token"].(string), nil); me.Status != 200 {
		t.Fatalf("new access token rejected %d", me.Status)
	}
	// single use
	if r3 := h.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": rt}); r3.Status != 401 {
		t.Fatalf("reused refresh token %d", r3.Status)
	}
	if r4 := h.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": ""}); r4.Status != 422 {
		t.Fatalf("empty refresh token %d", r4.Status)
	}
	// logout revokes the presented token
	access, newRT := b2["token"].(string), b2["refresh_token"].(string)
	if lo := h.do("POST", "/api/v1/auth/logout", access, map[string]any{"refresh_token": newRT}); lo.Status != http.StatusNoContent {
		t.Fatalf("logout %d", lo.Status)
	}
	if r5 := h.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": newRT}); r5.Status != 401 {
		t.Fatalf("refresh after logout %d", r5.Status)
	}
}

func TestRefreshTokenExpires(t *testing.T) {
	h := newHarness(t)
	h.api.RefreshTTL = time.Minute
	h.register("exp@example.com", "E")
	rt := login(h, "exp@example.com", "supersecret").json(t)["refresh_token"].(string)
	h.api.Now = func() time.Time { return testNow.Add(2 * time.Minute) }
	if r := h.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": rt}); r.Status != 401 {
		t.Fatalf("expired refresh token %d", r.Status)
	}
}

func TestPasswordResetFlow(t *testing.T) {
	h := newHarness(t)
	m := &captureMailer{}
	h.api.Mailer = m
	h.api.authLimiter.limit = 100 // this flow makes more than 10 auth calls
	h.register("reset@example.com", "R")
	oldRT := login(h, "reset@example.com", "supersecret").json(t)["refresh_token"].(string)

	// unknown and known e-mail answer identically
	a := h.do("POST", "/api/v1/auth/password/forgot", "", map[string]string{"email": "ghost@example.com"})
	b := h.do("POST", "/api/v1/auth/password/forgot", "", map[string]string{"email": "Reset@Example.com"})
	if a.Status != 202 || b.Status != 202 || string(a.Body) != string(b.Body) {
		t.Fatalf("forgot responses differ: %d %s / %d %s", a.Status, a.Body, b.Status, b.Body)
	}
	if m.last("ghost@example.com") != "" {
		t.Fatal("mail sent for unknown account")
	}
	tok := m.last("reset@example.com")
	if tok == "" {
		t.Fatal("no reset token sent")
	}
	if r := h.do("POST", "/api/v1/auth/password/forgot", "", map[string]string{"email": "nope"}); r.Status != 422 {
		t.Fatalf("invalid email %d", r.Status)
	}
	if r := h.do("POST", "/api/v1/auth/password/reset", "", map[string]string{"token": tok, "password": "short"}); r.Status != 422 {
		t.Fatalf("short password %d", r.Status)
	}
	if r := h.do("POST", "/api/v1/auth/password/reset", "", map[string]string{"token": "bogus", "password": "brandnewpass"}); r.Status != 400 {
		t.Fatalf("bogus token %d", r.Status)
	}
	if r := h.do("POST", "/api/v1/auth/password/reset", "", map[string]string{"token": tok, "password": "brandnewpass"}); r.Status != 204 {
		t.Fatalf("reset %d %s", r.Status, r.Body)
	}
	if r := h.do("POST", "/api/v1/auth/password/reset", "", map[string]string{"token": tok, "password": "anotherpass1"}); r.Status != 400 {
		t.Fatalf("reset token reused %d", r.Status)
	}
	if r := login(h, "reset@example.com", "supersecret"); r.Status != 401 {
		t.Fatalf("old password still works %d", r.Status)
	}
	if r := login(h, "reset@example.com", "brandnewpass"); r.Status != 200 {
		t.Fatalf("new password %d", r.Status)
	}
	if r := h.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": oldRT}); r.Status != 401 {
		t.Fatalf("refresh token survived password reset %d", r.Status)
	}
}

func TestPasswordResetOnlyNewestLinkWorks(t *testing.T) {
	h := newHarness(t)
	m := &captureMailer{}
	h.api.Mailer = m
	h.register("two@example.com", "T")
	h.do("POST", "/api/v1/auth/password/forgot", "", map[string]string{"email": "two@example.com"})
	first := m.last("two@example.com")
	h.do("POST", "/api/v1/auth/password/forgot", "", map[string]string{"email": "two@example.com"})
	if r := h.do("POST", "/api/v1/auth/password/reset", "", map[string]string{"token": first, "password": "brandnewpass"}); r.Status != 400 {
		t.Fatalf("superseded link accepted %d", r.Status)
	}
	h.api.Now = func() time.Time { return testNow.Add(2 * time.Hour) }
	if r := h.do("POST", "/api/v1/auth/password/reset", "", map[string]string{"token": m.last("two@example.com"), "password": "brandnewpass"}); r.Status != 400 {
		t.Fatalf("expired link accepted %d", r.Status)
	}
}

func TestChangePasswordAndProfile(t *testing.T) {
	h := newHarness(t)
	tok := h.register("chg@example.com", "Old Name")
	if r := h.do("PUT", "/api/v1/me/password", tok, map[string]string{"current_password": "wrongpass", "new_password": "x"}); r.Status != 422 {
		t.Fatalf("validation %d", r.Status)
	} else if f := r.json(t)["error"].(map[string]any)["fields"].(map[string]any); f["current_password"] == nil || f["new_password"] == nil {
		t.Fatalf("fields %v", f)
	}
	if r := h.do("PUT", "/api/v1/me/password", tok, map[string]string{"current_password": "supersecret", "new_password": "supersecret"}); r.Status != 422 {
		t.Fatalf("same password %d", r.Status)
	}
	if r := h.do("PUT", "/api/v1/me/password", tok, map[string]string{"current_password": "supersecret", "new_password": "evenbetter1"}); r.Status != 204 {
		t.Fatalf("change %d %s", r.Status, r.Body)
	}
	if r := login(h, "chg@example.com", "evenbetter1"); r.Status != 200 {
		t.Fatalf("login with new password %d", r.Status)
	}
	r := h.do("PATCH", "/api/v1/me", tok, map[string]string{"name": "  Yeni İsim "})
	if r.Status != 200 || r.json(t)["name"] != "Yeni İsim" {
		t.Fatalf("patch me %d %s", r.Status, r.Body)
	}
	long := make([]rune, 101)
	for i := range long {
		long[i] = 'a'
	}
	if r := h.do("PATCH", "/api/v1/me", tok, map[string]string{"name": string(long)}); r.Status != 422 {
		t.Fatalf("long name %d", r.Status)
	}
}

func TestRateLimitIgnoresSpoofedForwardedFor(t *testing.T) {
	h := newHarness(t)
	var last int
	for i := 0; i < 12; i++ {
		req, _ := http.NewRequest("POST", h.srv.URL+"/api/v1/auth/login", nil)
		req.Header.Set("X-Forwarded-For", "10.0.0."+string(rune('a'+i)))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		last = res.StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("spoofed X-Forwarded-For bypassed the limiter: %d", last)
	}
}
