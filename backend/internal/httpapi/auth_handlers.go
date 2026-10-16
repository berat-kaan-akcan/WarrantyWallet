package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/auth"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

type authResponse struct {
	Token            string      `json:"token"`
	ExpiresAt        time.Time   `json:"expires_at"`
	RefreshToken     string      `json:"refresh_token"`
	RefreshExpiresAt time.Time   `json:"refresh_expires_at"`
	User             domain.User `json:"user"`
}

// passwordProblem returns a validation message or "".
func passwordProblem(p string) string {
	switch {
	case utf8.RuneCountInString(p) < auth.MinPasswordLen:
		return "must be at least 8 characters"
	case len(p) > 128:
		return "must be at most 128 characters"
	case strings.TrimSpace(p) == "":
		return "must not be blank"
	}
	return ""
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	fields := map[string]string{}
	if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email || len(in.Email) > 254 {
		fields["email"] = "invalid email address"
	}
	if msg := passwordProblem(in.Password); msg != "" {
		fields["password"] = msg
	}
	if utf8.RuneCountInString(in.Name) > 100 {
		fields["name"] = "max 100 characters"
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", fields)
		return
	}
	hash, err := s.Hasher.Hash(in.Password)
	if err != nil {
		s.storeError(w, err)
		return
	}
	u := &domain.User{Email: in.Email, Name: in.Name, PasswordHash: hash, Role: domain.RoleUser}
	if slices.Contains(s.AdminEmails, in.Email) {
		u.Role = domain.RoleAdmin
	}
	if err := s.Store.CreateUser(r.Context(), u); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "email_taken", "an account with this email already exists", map[string]string{"email": "already registered"})
			return
		}
		s.storeError(w, err)
		return
	}
	s.respondWithToken(w, r, http.StatusCreated, *u)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	u, err := s.Store.GetUserByEmail(r.Context(), in.Email)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		s.storeError(w, err)
		return
	}
	// Same error for unknown email and wrong password (no account enumeration).
	if err != nil || !s.Hasher.Verify(in.Password, u.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect", nil)
		return
	}
	s.respondWithToken(w, r, http.StatusOK, u)
}

func (s *Server) respondWithToken(w http.ResponseWriter, r *http.Request, status int, u domain.User) {
	tok, exp, err := s.Tokens.Issue(u.ID, string(u.Role))
	if err != nil {
		s.storeError(w, err)
		return
	}
	plain, hash, err := auth.NewOpaqueToken()
	if err != nil {
		s.storeError(w, err)
		return
	}
	ttl := s.RefreshTTL
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	rt := &domain.AuthToken{UserID: u.ID, Kind: domain.TokenRefresh, TokenHash: hash, ExpiresAt: s.now().Add(ttl).UTC()}
	if err := s.Store.CreateAuthToken(r.Context(), rt); err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, status, authResponse{Token: tok, ExpiresAt: exp, RefreshToken: plain, RefreshExpiresAt: rt.ExpiresAt, User: u})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, userFrom(r.Context()))
}

// handleDeleteMe deletes the account and all data (KVKK/GDPR right to erasure).
func (s *Server) handleDeleteMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	s.deleteUserFiles(r.Context(), u.ID)
	if err := s.Store.DeleteUser(r.Context(), u.ID); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteUserFiles(ctx context.Context, userID string) {
	if s.Files == nil {
		return
	}
	products, err := s.Store.ListProducts(ctx, userID, storeFilterAll)
	if err != nil {
		return
	}
	for _, p := range products {
		s.deleteProductFiles(ctx, userID, p.ID)
	}
}
