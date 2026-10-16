package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/assist"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

// handleDiagnose runs the AI fault assistant for one of the user's products.
func (s *Server) handleDiagnose(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var in struct {
		Problem     domain.ProblemType `json:"problem"`
		Description string             `json:"description"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Problem == "" {
		in.Problem = domain.ProblemOther
	}
	if !domain.ValidProblem(in.Problem) {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"problem": "unknown problem type"})
		return
	}
	if utf8.RuneCountInString(in.Description) > 2000 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"description": "must be at most 2000 characters"})
		return
	}
	p, err := s.Store.GetProduct(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	v := s.view(p)
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	d, err := s.Assistant.Diagnose(ctx, assist.Input{
		Product:       p,
		Problem:       in.Problem,
		Description:   in.Description,
		UnderWarranty: domain.IsCovered(p.WarrantyEnd, s.today()),
		DaysLeft:      v.DaysLeft,
	})
	if errors.Is(err, assist.ErrEmptyInput) {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"description": "describe the problem (at least 5 characters)"})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "diagnosis failed", nil)
		return
	}
	s.Logger.Info("diagnosis", "provider", d.Provider, "model", d.Model, "problem", in.Problem, "warnings", len(d.Warnings))
	// Internal error details stay in the log; clients only learn that AI was unavailable.
	if len(d.Warnings) > 0 {
		d.Warnings = []string{"ai_unavailable"}
	}
	writeJSON(w, http.StatusOK, d)
}
