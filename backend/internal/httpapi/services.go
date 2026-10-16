package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/pdf"
)

type serviceInput struct {
	Problem     *domain.ProblemType   `json:"problem"`
	Description *string               `json:"description"`
	ServiceName *string               `json:"service_name"`
	ServiceDate *domain.Date          `json:"service_date"`
	Status      *domain.ServiceStatus `json:"status"`
	Notes       *string               `json:"notes"`
}

func (in serviceInput) apply(r *domain.ServiceRecord, today domain.Date) map[string]string {
	errs := map[string]string{}
	if in.Problem != nil {
		r.Problem = *in.Problem
	}
	if in.Description != nil {
		r.Description = strings.TrimSpace(*in.Description)
	}
	if in.ServiceName != nil {
		r.ServiceName = strings.TrimSpace(*in.ServiceName)
	}
	if in.ServiceDate != nil {
		r.ServiceDate = *in.ServiceDate
	}
	if in.Status != nil {
		r.Status = *in.Status
	}
	if in.Notes != nil {
		r.Notes = strings.TrimSpace(*in.Notes)
	}
	if !domain.ValidProblem(r.Problem) {
		errs["problem"] = "wont_turn_on|screen_lines|physical_damage|other"
	}
	if !domain.ValidServiceStatus(r.Status) {
		errs["status"] = "open|in_service|resolved|cancelled"
	}
	if utf8.RuneCountInString(r.Description) > 2000 {
		errs["description"] = "max 2000 characters"
	}
	if utf8.RuneCountInString(r.Notes) > 2000 {
		errs["notes"] = "max 2000 characters"
	}
	if utf8.RuneCountInString(r.ServiceName) > 200 {
		errs["service_name"] = "max 200 characters"
	}
	if !r.ServiceDate.IsZero() && (r.ServiceDate.Before(domain.MinPurchaseDate) || r.ServiceDate.After(today.AddDays(366))) {
		errs["service_date"] = "must be between 1990-01-01 and one year from today"
	}
	return errs
}

// serviceResponse returns the record together with the warranty checklist ("Ürünüm Arızalandı").
type serviceResponse struct {
	domain.ServiceRecord
	Checklist []pdf.ChecklistItem `json:"checklist,omitempty"`
}

func (s *Server) handleCreateServiceRecord(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	p, err := s.Store.GetProduct(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	var in serviceInput
	if !decodeJSON(w, r, &in) {
		return
	}
	rec := domain.ServiceRecord{UserID: u.ID, ProductID: p.ID, Status: domain.ServiceOpen,
		UnderWarranty: domain.IsCovered(p.WarrantyEnd, s.today())}
	if errs := in.apply(&rec, s.today()); len(errs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid service record", errs)
		return
	}
	if err := s.Store.CreateServiceRecord(r.Context(), &rec); err != nil {
		s.storeError(w, err)
		return
	}
	resp := serviceResponse{ServiceRecord: rec}
	if rec.UnderWarranty {
		docs, _ := s.Store.ListDocuments(r.Context(), u.ID, p.ID)
		resp.Checklist = pdf.Checklist(p, docs)
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleListProductServiceRecords(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	if _, err := s.Store.GetProduct(r.Context(), u.ID, r.PathValue("id")); err != nil {
		s.storeError(w, err)
		return
	}
	recs, err := s.Store.ListServiceRecords(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": recs})
}

// serviceHistoryItem enriches records with product name for the history list.
type serviceHistoryItem struct {
	domain.ServiceRecord
	ProductName string `json:"product_name"`
}

func (s *Server) handleListServiceRecords(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	recs, err := s.Store.ListServiceRecords(r.Context(), u.ID, "")
	if err != nil {
		s.storeError(w, err)
		return
	}
	status := r.URL.Query().Get("status")
	names := map[string]string{}
	out := make([]serviceHistoryItem, 0, len(recs))
	for _, rec := range recs {
		if status != "" && string(rec.Status) != status {
			continue
		}
		name, ok := names[rec.ProductID]
		if !ok {
			if p, err := s.Store.GetProduct(r.Context(), u.ID, rec.ProductID); err == nil {
				name = p.Name
			}
			names[rec.ProductID] = name
		}
		out = append(out, serviceHistoryItem{ServiceRecord: rec, ProductName: name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleGetServiceRecord(w http.ResponseWriter, r *http.Request) {
	rec, err := s.Store.GetServiceRecord(r.Context(), userFrom(r.Context()).ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleUpdateServiceRecord(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	rec, err := s.Store.GetServiceRecord(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	var in serviceInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if errs := in.apply(&rec, s.today()); len(errs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid service record", errs)
		return
	}
	if err := s.Store.UpdateServiceRecord(r.Context(), &rec); err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleChecklist(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	p, err := s.Store.GetProduct(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	docs, err := s.Store.ListDocuments(r.Context(), u.ID, p.ID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	v := s.view(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"under_warranty": domain.IsCovered(p.WarrantyEnd, s.today()),
		"status":         v.Status,
		"days_left":      v.DaysLeft,
		"items":          pdf.Checklist(p, docs),
	})
}

func (s *Server) handleServicePack(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	p, err := s.Store.GetProduct(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	var sr *domain.ServiceRecord
	if id := r.URL.Query().Get("service_record_id"); id != "" {
		rec, err := s.Store.GetServiceRecord(r.Context(), u.ID, id)
		if err != nil || rec.ProductID != p.ID {
			writeError(w, http.StatusNotFound, "not_found", "service record not found", nil)
			return
		}
		sr = &rec
	}
	docs, err := s.Store.ListDocuments(r.Context(), u.ID, p.ID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	atts := make([]pdf.Attachment, 0, len(docs))
	for _, d := range docs {
		if sr != nil && d.Type == domain.DocPhoto && d.ServiceRecordID != "" && d.ServiceRecordID != sr.ID {
			continue
		}
		atts = append(atts, pdf.Attachment{Doc: d, Data: s.loadFile(r.Context(), d.FileKey)})
	}
	owner := u.Name
	if owner == "" {
		owner = u.Email
	}
	b, err := pdf.ServicePack(p, sr, atts, s.today(), owner)
	if err != nil {
		s.storeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="servis-paketi-%s.pdf"`, p.ID[:8]))
	_, _ = io.Copy(w, bytes.NewReader(b))
}

func (s *Server) loadFile(ctx context.Context, key string) []byte {
	if s.Files == nil {
		return nil
	}
	rc, err := s.Files.Open(ctx, key)
	if err != nil {
		return nil
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, 20<<20))
	if err != nil {
		return nil
	}
	return b
}
