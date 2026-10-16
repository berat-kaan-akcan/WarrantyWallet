package httpapi

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	logs, _ := s.Store.ListExtractionLogs(r.Context(), false, 1000)
	ok, failed := 0, 0
	providers := map[string]int{}
	for _, l := range logs {
		providers[l.Provider]++
		if l.Success {
			ok++
		} else {
			failed++
		}
	}
	products := 0
	for _, u := range users {
		ps, _ := s.Store.ListProducts(r.Context(), u.ID, storeFilterAll)
		products += len(ps)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users": len(users), "products": products,
		"extractions": map[string]any{"ok": ok, "failed": failed, "by_provider": providers, "sample_size": len(logs)},
	})
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role domain.Role `json:"role"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Role != domain.RoleUser && in.Role != domain.RoleAdmin {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid role", map[string]string{"role": "user|admin"})
		return
	}
	id := r.PathValue("id")
	if id == userFrom(r.Context()).ID && in.Role != domain.RoleAdmin {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "you cannot demote yourself", nil)
		return
	}
	if err := s.Store.SetUserRole(r.Context(), id, in.Role); err != nil {
		s.storeError(w, err)
		return
	}
	u, err := s.Store.GetUserByID(r.Context(), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == userFrom(r.Context()).ID {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "use DELETE /me to delete your own account", nil)
		return
	}
	if _, err := s.Store.GetUserByID(r.Context(), id); err != nil {
		s.storeError(w, err)
		return
	}
	s.deleteUserFiles(r.Context(), id)
	if err := s.Store.DeleteUser(r.Context(), id); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminExtractionLogs(w http.ResponseWriter, r *http.Request) {
	only := r.URL.Query().Get("failures") == "1" || r.URL.Query().Get("failures") == "true"
	logs, err := s.Store.ListExtractionLogs(r.Context(), only, 200)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": logs})
}

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

func (s *Server) handleAdminUpsertCategory(w http.ResponseWriter, r *http.Request) {
	var c domain.Category
	if !decodeJSON(w, r, &c) {
		return
	}
	c.Slug = strings.TrimSpace(c.Slug)
	c.NameTR = strings.TrimSpace(c.NameTR)
	errs := map[string]string{}
	if !slugRe.MatchString(c.Slug) {
		errs["slug"] = "lowercase letters, digits, underscore (2-40)"
	}
	if c.NameTR == "" || len(c.NameTR) > 60 {
		errs["name_tr"] = "required, max 60 characters"
	}
	if len(errs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid category", errs)
		return
	}
	if err := s.Store.UpsertCategory(r.Context(), c); err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleAdminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "other" {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "the 'other' category cannot be deleted", nil)
		return
	}
	if err := s.Store.DeleteCategory(r.Context(), slug); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
