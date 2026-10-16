package httpapi

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/auth"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

// PasswordResetTTL is how long a password reset link stays valid.
const PasswordResetTTL = time.Hour

// handleRefresh rotates a refresh token: the presented token is consumed (single use) and a
// new access + refresh pair is returned.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.RefreshToken) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"refresh_token": "required"})
		return
	}
	t, err := s.Store.ConsumeAuthToken(r.Context(), domain.TokenRefresh, auth.HashOpaqueToken(strings.TrimSpace(in.RefreshToken)), s.now())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "refresh token is invalid, expired or already used; please sign in again", nil)
			return
		}
		s.storeError(w, err)
		return
	}
	u, err := s.Store.GetUserByID(r.Context(), t.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "user no longer exists", nil)
		return
	}
	s.respondWithToken(w, r, http.StatusOK, u)
}

// handleLogout revokes the given refresh token (idempotent).
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
		All          bool   `json:"all"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	u := userFrom(r.Context())
	if in.All {
		if err := s.Store.RevokeAuthTokens(r.Context(), u.ID, domain.TokenRefresh, s.now()); err != nil {
			s.storeError(w, err)
			return
		}
	} else if in.RefreshToken != "" {
		t, err := s.Store.ConsumeAuthToken(r.Context(), domain.TokenRefresh, auth.HashOpaqueToken(strings.TrimSpace(in.RefreshToken)), s.now())
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			s.storeError(w, err)
			return
		}
		if err == nil && t.UserID != u.ID {
			s.Logger.Warn("logout with another user's refresh token", "user", u.ID)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleForgotPassword always answers 202 so the endpoint cannot be used to discover accounts.
func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"email": "invalid email address"})
		return
	}
	accepted := map[string]string{"status": "accepted", "message": "if an account exists, a reset link has been sent"}
	u, err := s.Store.GetUserByEmail(r.Context(), email)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			s.Logger.Error("forgot password lookup", "err", err)
		}
		writeJSON(w, http.StatusAccepted, accepted)
		return
	}
	plain, hash, err := auth.NewOpaqueToken()
	if err == nil {
		// Only the newest reset link works.
		_ = s.Store.RevokeAuthTokens(r.Context(), u.ID, domain.TokenPasswordReset, s.now())
		err = s.Store.CreateAuthToken(r.Context(), &domain.AuthToken{UserID: u.ID, Kind: domain.TokenPasswordReset,
			TokenHash: hash, ExpiresAt: s.now().Add(PasswordResetTTL).UTC()})
	}
	if err == nil && s.Mailer != nil {
		err = s.Mailer.SendPasswordReset(r.Context(), u.Email, u.Name, plain)
	}
	if err != nil {
		s.Logger.Error("password reset", "err", err)
	}
	writeJSON(w, http.StatusAccepted, accepted)
}

// handleResetPassword sets a new password with a single-use reset token and signs the user
// out everywhere (all refresh tokens revoked).
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	fields := map[string]string{}
	if strings.TrimSpace(in.Token) == "" {
		fields["token"] = "required"
	}
	if msg := passwordProblem(in.Password); msg != "" {
		fields["password"] = msg
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", fields)
		return
	}
	t, err := s.Store.ConsumeAuthToken(r.Context(), domain.TokenPasswordReset, auth.HashOpaqueToken(strings.TrimSpace(in.Token)), s.now())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "invalid_reset_token", "reset link is invalid or expired", nil)
			return
		}
		s.storeError(w, err)
		return
	}
	if !s.setPassword(w, r, t.UserID, in.Password) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleChangePassword changes the password of the signed-in user.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	u := userFrom(r.Context())
	fields := map[string]string{}
	if !s.Hasher.Verify(in.CurrentPassword, u.PasswordHash) {
		fields["current_password"] = "incorrect password"
	}
	if msg := passwordProblem(in.NewPassword); msg != "" {
		fields["new_password"] = msg
	} else if in.NewPassword == in.CurrentPassword {
		fields["new_password"] = "must differ from the current password"
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", fields)
		return
	}
	if !s.setPassword(w, r, u.ID, in.NewPassword) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setPassword(w http.ResponseWriter, r *http.Request, userID, password string) bool {
	hash, err := s.Hasher.Hash(password)
	if err == nil {
		err = s.Store.SetUserPassword(r.Context(), userID, hash)
	}
	if err == nil {
		err = s.Store.RevokeAuthTokens(r.Context(), userID, domain.TokenRefresh, s.now())
	}
	if err != nil {
		s.storeError(w, err)
		return false
	}
	return true
}

// handleUpdateMe updates profile fields (currently the display name).
func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name *string `json:"name"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	u := userFrom(r.Context())
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if utf8.RuneCountInString(name) > 100 {
			writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"name": "max 100 characters"})
			return
		}
		if err := s.Store.UpdateUserProfile(r.Context(), u.ID, name); err != nil {
			s.storeError(w, err)
			return
		}
		u.Name = name
	}
	writeJSON(w, http.StatusOK, u)
}
