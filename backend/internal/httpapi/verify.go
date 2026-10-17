package httpapi

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/auth"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

// EmailVerifyTTL is how long a verification link stays valid.
const EmailVerifyTTL = 24 * time.Hour

// sendVerification issues a new single-use link (older ones stop working) and mails it.
// Failures are logged: registration must not fail because the mail relay is down.
func (s *Server) sendVerification(ctx context.Context, u domain.User) {
	plain, hash, err := auth.NewOpaqueToken()
	if err == nil {
		_ = s.Store.RevokeAuthTokens(ctx, u.ID, domain.TokenEmailVerify, s.now())
		err = s.Store.CreateAuthToken(ctx, &domain.AuthToken{UserID: u.ID, Kind: domain.TokenEmailVerify,
			TokenHash: hash, ExpiresAt: s.now().Add(EmailVerifyTTL).UTC()})
	}
	if err == nil && s.Mailer != nil {
		base := strings.TrimRight(s.PublicBaseURL, "/")
		if base == "" {
			base = "http://localhost:8080"
		}
		err = s.Mailer.SendEmailVerification(ctx, u.Email, u.Name, base+"/api/v1/auth/email/verify?token="+url.QueryEscape(plain))
	}
	if err != nil {
		s.Logger.Error("email verification", "err", err)
	}
}

// verifyEmailToken consumes a token and marks the address verified.
func (s *Server) verifyEmailToken(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return domain.ErrNotFound
	}
	t, err := s.Store.ConsumeAuthToken(ctx, domain.TokenEmailVerify, auth.HashOpaqueToken(token), s.now())
	if err != nil {
		return err
	}
	return s.Store.MarkEmailVerified(ctx, t.UserID, s.now())
}

// handleResendVerification mails a fresh link to the signed-in user.
func (s *Server) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	if u.EmailVerifiedAt != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_verified"})
		return
	}
	s.sendVerification(r.Context(), u)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
}

// handleVerifyEmailJSON is the app-facing variant: POST {"token": "..."}.
func (s *Server) handleVerifyEmailJSON(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.verifyEmailToken(r.Context(), in.Token); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "invalid_verification_token", "verification link is invalid or expired", nil)
			return
		}
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

var verifyPage = template.Must(template.New("v").Parse(`<!doctype html>
<html lang="tr"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Warranty Wallet</title>
<style>body{font:17px/1.5 system-ui,sans-serif;max-width:32rem;margin:3rem auto;padding:0 1rem;color:#1d2330}
h1{font-size:1.4rem;color:{{.Color}}}</style></head>
<body><h1>{{.Title}}</h1><p>{{.Text}}</p></body></html>`))

// handleVerifyEmailLink is opened from the e-mail: GET ?token=... and answers with a small page.
func (s *Server) handleVerifyEmailLink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	data := struct{ Title, Text, Color string }{"E-posta adresiniz doğrulandı",
		"Teşekkürler! Uygulamaya dönebilirsiniz.", "#2e7d32"}
	status := http.StatusOK
	if err := s.verifyEmailToken(r.Context(), r.URL.Query().Get("token")); err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			s.Logger.Error("verify email", "err", err)
		}
		status = http.StatusBadRequest
		data = struct{ Title, Text, Color string }{"Bağlantı geçersiz",
			"Doğrulama bağlantısı geçersiz, kullanılmış veya süresi dolmuş. Uygulamadan yeni bir bağlantı isteyebilirsiniz.", "#c62828"}
	}
	w.WriteHeader(status)
	_ = verifyPage.Execute(w, data)
}
