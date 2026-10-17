package httpapi

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/auth"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

// FamilyInviteTTL is how long an invite code can be used (once).
const FamilyInviteTTL = 7 * 24 * time.Hour

// inviteAlphabet avoids look-alike characters (0/O, 1/I/L) so codes can be read out loud.
const inviteAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// newInviteCode returns a code like "K7QM-2XRP" (8 symbols ≈ 39 bits; single use, 7 days, rate limited).
func newInviteCode() (string, error) {
	out := make([]byte, 0, 9)
	buf := make([]byte, 16)
	for len(out) < 9 {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, c := range buf {
			if len(out) == 9 {
				break
			}
			if int(c) >= 256-256%len(inviteAlphabet) { // rejection sampling: no modulo bias
				continue
			}
			if len(out) == 4 {
				out = append(out, '-')
			}
			out = append(out, inviteAlphabet[int(c)%len(inviteAlphabet)])
		}
	}
	return string(out), nil
}

// normalizeInviteCode accepts lower case, spaces and dashes.
func normalizeInviteCode(c string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(c)))
}

func inviteHash(code string) string {
	return auth.HashOpaqueToken("family:" + normalizeInviteCode(code))
}

type familyView struct {
	Family  *domain.Family            `json:"family"`
	Role    domain.FamilyRole         `json:"role,omitempty"`
	Members []domain.FamilyMemberInfo `json:"members"`
}

// familyOf returns the caller's family and role; ok=false when the user is in none.
func (s *Server) familyOf(ctx context.Context, userID string) (domain.Family, []domain.FamilyMemberInfo, domain.FamilyRole, bool, error) {
	f, err := s.Store.GetFamilyForUser(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return f, nil, "", false, nil
	}
	if err != nil {
		return f, nil, "", false, err
	}
	ms, err := s.Store.ListFamilyMembers(ctx, f.ID)
	if err != nil {
		return f, nil, "", false, err
	}
	role := domain.FamilyMember
	if f.OwnerID == userID {
		role = domain.FamilyOwner
	}
	return f, ms, role, true, nil
}

func (s *Server) writeFamily(w http.ResponseWriter, r *http.Request, status int) {
	f, ms, role, ok, err := s.familyOf(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if !ok {
		writeJSON(w, status, familyView{Members: []domain.FamilyMemberInfo{}})
		return
	}
	writeJSON(w, status, familyView{Family: &f, Role: role, Members: ms})
}

func (s *Server) handleGetFamily(w http.ResponseWriter, r *http.Request) {
	s.writeFamily(w, r, http.StatusOK)
}

func familyName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &body) {
		return "", false
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || utf8.RuneCountInString(name) > 60 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid family name", map[string]string{"name": "1-60 characters"})
		return "", false
	}
	return name, true
}

func (s *Server) handleCreateFamily(w http.ResponseWriter, r *http.Request) {
	name, ok := familyName(w, r)
	if !ok {
		return
	}
	f := &domain.Family{Name: name, OwnerID: userFrom(r.Context()).ID}
	if err := s.Store.CreateFamily(r.Context(), f); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "already_in_family", "leave your current family first", nil)
			return
		}
		s.storeError(w, err)
		return
	}
	s.writeFamily(w, r, http.StatusCreated)
}

// requireFamilyOwner loads the caller's family and checks ownership, writing the error otherwise.
func (s *Server) requireFamilyOwner(w http.ResponseWriter, r *http.Request) (domain.Family, bool) {
	u := userFrom(r.Context())
	f, err := s.Store.GetFamilyForUser(r.Context(), u.ID)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_in_family", "you are not in a family", nil)
		return f, false
	}
	if err != nil {
		s.storeError(w, err)
		return f, false
	}
	if f.OwnerID != u.ID {
		writeError(w, http.StatusForbidden, "forbidden", "only the family owner can do this", nil)
		return f, false
	}
	return f, true
}

func (s *Server) handleRenameFamily(w http.ResponseWriter, r *http.Request) {
	f, ok := s.requireFamilyOwner(w, r)
	if !ok {
		return
	}
	name, ok := familyName(w, r)
	if !ok {
		return
	}
	if err := s.Store.RenameFamily(r.Context(), f.ID, name); err != nil {
		s.storeError(w, err)
		return
	}
	s.writeFamily(w, r, http.StatusOK)
}

// handleCreateInvite issues a single-use code the owner shares with a family member.
func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	f, ok := s.requireFamilyOwner(w, r)
	if !ok {
		return
	}
	if ms, err := s.Store.ListFamilyMembers(r.Context(), f.ID); err == nil && len(ms) >= domain.MaxFamilyMembers {
		writeError(w, http.StatusConflict, "family_full", "a family can have at most 8 members", nil)
		return
	}
	code, err := newInviteCode()
	if err != nil {
		s.storeError(w, err)
		return
	}
	exp := s.now().Add(FamilyInviteTTL).UTC()
	t := &domain.AuthToken{UserID: f.OwnerID, Kind: domain.TokenFamilyInvite, TokenHash: inviteHash(code), ExpiresAt: exp}
	if err := s.Store.CreateAuthToken(r.Context(), t); err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"code": code, "expires_at": exp})
}

func (s *Server) handleJoinFamily(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	u := userFrom(r.Context())
	if len(normalizeInviteCode(body.Code)) != 8 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid code", map[string]string{"code": "8 characters, e.g. K7QM-2XRP"})
		return
	}
	// Check first so a member of another family does not burn the code.
	if _, err := s.Store.GetFamilyForUser(r.Context(), u.ID); err == nil {
		writeError(w, http.StatusConflict, "already_in_family", "leave your current family first", nil)
		return
	}
	invalid := func() {
		writeError(w, http.StatusBadRequest, "invalid_invite_code", "the code is wrong, expired or already used", nil)
	}
	t, err := s.Store.ConsumeAuthToken(r.Context(), domain.TokenFamilyInvite, inviteHash(body.Code), s.now())
	if err != nil {
		invalid()
		return
	}
	f, err := s.Store.GetFamilyForUser(r.Context(), t.UserID)
	if err != nil || f.OwnerID != t.UserID { // the owner left or handed over the family since
		invalid()
		return
	}
	if ms, err := s.Store.ListFamilyMembers(r.Context(), f.ID); err == nil && len(ms) >= domain.MaxFamilyMembers {
		writeError(w, http.StatusConflict, "family_full", "a family can have at most 8 members", nil)
		return
	}
	if err := s.Store.AddFamilyMember(r.Context(), f.ID, u.ID); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "already_in_family", "leave your current family first", nil)
			return
		}
		s.storeError(w, err)
		return
	}
	s.writeFamily(w, r, http.StatusOK)
}

// handleRemoveFamilyMember: a member leaves (id = self), or the owner removes someone.
// When the owner leaves, the family is dissolved.
func (s *Server) handleRemoveFamilyMember(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	target := r.PathValue("id")
	f, err := s.Store.GetFamilyForUser(r.Context(), u.ID)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_in_family", "you are not in a family", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch {
	case target == u.ID && f.OwnerID == u.ID:
		err = s.Store.DeleteFamily(r.Context(), f.ID)
		if err == nil {
			_ = s.Store.RevokeAuthTokens(r.Context(), u.ID, domain.TokenFamilyInvite, s.now())
		}
	case target == u.ID || f.OwnerID == u.ID:
		err = s.Store.RemoveFamilyMember(r.Context(), f.ID, target)
	default:
		writeError(w, http.StatusForbidden, "forbidden", "only the family owner can remove members", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type familyProduct struct {
	ProductView
	Owner familyOwner `json:"owner"`
}

type familyOwner struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// handleFamilyProducts lists every member's products (read-only), soonest warranty end first.
func (s *Server) handleFamilyProducts(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	status := domain.WarrantyStatus(r.URL.Query().Get("status"))
	switch status {
	case "", domain.StatusActive, domain.StatusExpiringSoon, domain.StatusExpired, domain.StatusUnknown:
	default:
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid filter", map[string]string{"status": "active|expiring_soon|expired|unknown"})
		return
	}
	_, ms, _, ok, err := s.familyOf(r.Context(), u.ID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_in_family", "you are not in a family", nil)
		return
	}
	items := []familyProduct{}
	for _, m := range ms {
		ps, err := s.Store.ListProducts(r.Context(), m.UserID, storeFilterAll)
		if err != nil {
			s.storeError(w, err)
			return
		}
		name := m.Name
		if name == "" {
			name = strings.SplitN(m.Email, "@", 2)[0]
		}
		for _, p := range ps {
			v := s.view(p)
			if status != "" && v.Status != status {
				continue
			}
			items = append(items, familyProduct{ProductView: v, Owner: familyOwner{ID: m.UserID, Name: name}})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].DaysLeft, items[j].DaysLeft
		if (a == nil) != (b == nil) {
			return b == nil
		}
		if a != nil && *a != *b {
			return *a < *b
		}
		return items[i].Name < items[j].Name
	})
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
