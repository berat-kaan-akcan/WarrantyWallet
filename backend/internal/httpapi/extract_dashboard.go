package httpapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/extract"
)

// handleExtract accepts multipart (image + optional text) or JSON {"text": "..."} and returns
// suggested product fields plus the computed warranty end date. Nothing is saved.
func (s *Server) handleExtract(w http.ResponseWriter, r *http.Request) {
	if s.Extractor == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "extraction disabled", nil)
		return
	}
	var in extract.Input
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch ct {
	case "multipart/form-data":
		r.Body = http.MaxBytesReader(w, r.Body, s.MaxUpload+1<<20)
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid multipart body", nil)
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()
		data, mt, _, ok := s.readUpload(w, r, "image", false)
		if !ok {
			return
		}
		in.Image, in.MIME = data, mt
		in.Text = r.FormValue("text")
	default:
		var body struct {
			Text string `json:"text"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		in.Text = body.Text
	}
	if len(in.Text) > 50000 {
		in.Text = in.Text[:50000]
	}
	if strings.TrimSpace(in.Text) == "" && len(in.Image) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "provide an image or OCR text", map[string]string{"image": "required without text"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	res, err := s.Extractor.Extract(ctx, in)
	u := userFrom(r.Context())
	logEntry := &domain.ExtractionLog{UserID: u.ID, Provider: res.Provider, Success: err == nil && len(res.Missing) < 4, RawText: truncate(in.Text, 4000)}
	if err != nil {
		logEntry.Error = err.Error()
	} else if len(res.Warnings) > 0 {
		logEntry.Error = strings.Join(res.Warnings, "; ")
	}
	if logEntry.Provider == "" {
		logEntry.Provider = "none"
	}
	if lerr := s.Store.CreateExtractionLog(r.Context(), logEntry); lerr != nil {
		s.Logger.Warn("extraction log failed", "err", lerr)
	}
	if err != nil {
		if errors.Is(err, extract.ErrEmptyInput) {
			writeError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error(), nil)
			return
		}
		writeError(w, http.StatusBadGateway, "extraction_failed", "could not read the receipt; please fill in manually", nil)
		return
	}
	months := res.Fields.WarrantyMonths
	suggested := months
	if suggested == 0 {
		suggested = domain.DefaultWarrantyMonths
	}
	var end domain.Date
	if pd, perr := domain.ParseDate(res.Fields.PurchaseDate); perr == nil {
		end = domain.WarrantyEnd(pd, suggested)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fields":                    res.Fields,
		"provider":                  res.Provider,
		"missing":                   res.Missing,
		"warnings":                  res.Warnings,
		"raw_text":                  res.RawText,
		"suggested_warranty_months": suggested,
		"warranty_end":              end,
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type categoryStat struct {
	Category string  `json:"category"`
	Name     string  `json:"name"`
	Count    int     `json:"count"`
	Total    float64 `json:"total"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	products, err := s.Store.ListProducts(r.Context(), u.ID, storeFilterAll)
	if err != nil {
		s.storeError(w, err)
		return
	}
	cats, _ := s.Store.ListCategories(r.Context())
	names := map[string]string{}
	for _, c := range cats {
		names[c.Slug] = c.NameTR
	}
	var active, soon, expired, unknown int
	spent := map[string]float64{}
	byCat := map[string]*categoryStat{}
	var upcoming []ProductView
	for _, p := range products {
		v := s.view(p)
		switch v.Status {
		case domain.StatusActive:
			active++
		case domain.StatusExpiringSoon:
			active++
			soon++
		case domain.StatusExpired:
			expired++
		default:
			unknown++
		}
		if v.Status == domain.StatusActive || v.Status == domain.StatusExpiringSoon {
			if v.DaysLeft != nil && *v.DaysLeft <= 90 {
				upcoming = append(upcoming, v)
			}
		}
		spent[p.Currency] = math.Round((spent[p.Currency]+p.Price)*100) / 100
		cs, ok := byCat[p.Category]
		if !ok {
			name := names[p.Category]
			if name == "" {
				name = p.Category
			}
			cs = &categoryStat{Category: p.Category, Name: name}
			byCat[p.Category] = cs
		}
		cs.Count++
		cs.Total = math.Round((cs.Total+p.Price)*100) / 100
	}
	sort.Slice(upcoming, func(i, j int) bool { return *upcoming[i].DaysLeft < *upcoming[j].DaysLeft })
	if len(upcoming) > 5 {
		upcoming = upcoming[:5]
	}
	catList := make([]categoryStat, 0, len(byCat))
	for _, c := range byCat {
		catList = append(catList, *c)
	}
	sort.Slice(catList, func(i, j int) bool {
		if catList[i].Count == catList[j].Count {
			return catList[i].Category < catList[j].Category
		}
		return catList[i].Count > catList[j].Count
	})
	recs, _ := s.Store.ListServiceRecords(r.Context(), u.ID, "")
	open := 0
	for _, rec := range recs {
		if rec.Status == domain.ServiceOpen || rec.Status == domain.ServiceInService {
			open++
		}
	}
	if upcoming == nil {
		upcoming = []ProductView{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"greeting":             greeting(u, s.now()),
		"total_products":       len(products),
		"active_warranties":    active,
		"expiring_soon":        soon,
		"expired":              expired,
		"unknown":              unknown,
		"total_spent":          spent,
		"by_category":          catList,
		"upcoming":             upcoming,
		"open_service_records": open,
		"service_records":      len(recs),
	})
}

func greeting(u domain.User, now time.Time) string {
	h := now.In(time.FixedZone("TRT", 3*3600)).Hour()
	g := "Merhaba"
	switch {
	case h >= 5 && h < 12:
		g = "Günaydın"
	case h >= 18 || h < 5:
		g = "İyi akşamlar"
	}
	name := strings.Fields(u.Name)
	if len(name) > 0 {
		return fmt.Sprintf("%s, %s", g, name[0])
	}
	return g
}
