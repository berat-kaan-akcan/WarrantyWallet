package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/notify"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store"
)

var storeFilterAll = store.ProductFilter{}

// ProductView adds derived warranty info to a product.
type ProductView struct {
	domain.Product
	Status   domain.WarrantyStatus `json:"status"`
	DaysLeft *int                  `json:"days_left"`
}

func (s *Server) view(p domain.Product) ProductView {
	today := s.today()
	v := ProductView{Product: p, Status: domain.StatusOf(p.WarrantyEnd, today)}
	if !p.WarrantyEnd.IsZero() {
		d := domain.DaysLeft(p.WarrantyEnd, today)
		v.DaysLeft = &d
	}
	return v
}

// productInput is what clients send; warranty_end is always computed server-side.
type productInput struct {
	Name           string      `json:"name"`
	Brand          string      `json:"brand"`
	Model          string      `json:"model"`
	Category       string      `json:"category"`
	Seller         string      `json:"seller"`
	PurchaseDate   domain.Date `json:"purchase_date"`
	Price          float64     `json:"price"`
	Currency       string      `json:"currency"`
	WarrantyMonths *int        `json:"warranty_months"`
	SerialNumber   string      `json:"serial_number"`
	Notes          string      `json:"notes"`
}

func (in productInput) apply(p *domain.Product) {
	p.Name, p.Brand, p.Model, p.Category, p.Seller = in.Name, in.Brand, in.Model, in.Category, in.Seller
	p.PurchaseDate, p.Price, p.Currency = in.PurchaseDate, in.Price, in.Currency
	p.SerialNumber, p.Notes = in.SerialNumber, in.Notes
	if in.WarrantyMonths != nil {
		p.WarrantyMonths = *in.WarrantyMonths
	} else {
		p.WarrantyMonths = domain.DefaultWarrantyMonths
	}
	p.Normalize()
}

func (s *Server) validateProduct(ctx context.Context, p *domain.Product) map[string]string {
	errs := p.Validate(s.today())
	if _, bad := errs["category"]; !bad {
		cats, err := s.Store.ListCategories(ctx)
		if err == nil {
			ok := false
			for _, c := range cats {
				if c.Slug == p.Category {
					ok = true
					break
				}
			}
			if !ok {
				errs["category"] = "unknown category"
			}
		}
	}
	return errs
}

func (s *Server) handleCreateProduct(w http.ResponseWriter, r *http.Request) {
	var in productInput
	if !decodeJSON(w, r, &in) {
		return
	}
	u := userFrom(r.Context())
	p := domain.Product{UserID: u.ID}
	in.apply(&p)
	if errs := s.validateProduct(r.Context(), &p); len(errs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid product", errs)
		return
	}
	if err := s.Store.CreateProduct(r.Context(), &p); err != nil {
		s.storeError(w, err)
		return
	}
	s.syncReminders(r.Context(), p)
	writeJSON(w, http.StatusCreated, s.view(p))
}

func (s *Server) syncReminders(ctx context.Context, p domain.Product) {
	if err := notify.Sync(ctx, s.Store, p, s.now()); err != nil {
		s.Logger.Warn("reminder sync failed", "product", p.ID, "err", err)
	}
}

func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	q := r.URL.Query()
	f := store.ProductFilter{Category: q.Get("category"), Query: strings.TrimSpace(q.Get("q"))}
	status := domain.WarrantyStatus(q.Get("status"))
	switch status {
	case "", domain.StatusActive, domain.StatusExpiringSoon, domain.StatusExpired, domain.StatusUnknown:
	default:
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid filter", map[string]string{"status": "active|expiring_soon|expired|unknown"})
		return
	}
	if utf8.RuneCountInString(f.Query) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid filter", map[string]string{"q": "max 100 characters"})
		return
	}
	products, err := s.Store.ListProducts(r.Context(), u.ID, f)
	if err != nil {
		s.storeError(w, err)
		return
	}
	out := make([]ProductView, 0, len(products))
	for _, p := range products {
		v := s.view(p)
		if status != "" && v.Status != status {
			continue
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "count": len(out)})
}

func (s *Server) handleExpiring(w http.ResponseWriter, r *http.Request) {
	days := 90
	if v, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && v > 0 && v <= 3650 {
		days = v
	}
	u := userFrom(r.Context())
	today := s.today()
	products, err := s.Store.ListProducts(r.Context(), u.ID, store.ProductFilter{EndFrom: today, EndTo: today.AddDays(days)})
	if err != nil {
		s.storeError(w, err)
		return
	}
	sort.Slice(products, func(i, j int) bool { return products[i].WarrantyEnd.Before(products[j].WarrantyEnd) })
	out := make([]ProductView, 0, len(products))
	for _, p := range products {
		out = append(out, s.view(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "count": len(out), "days": days})
}

func (s *Server) handleGetProduct(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.GetProduct(r.Context(), userFrom(r.Context()).ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.view(p))
}

func (s *Server) handleUpdateProduct(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	p, err := s.Store.GetProduct(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	var in productInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.WarrantyMonths == nil {
		wm := p.WarrantyMonths
		in.WarrantyMonths = &wm
	}
	in.apply(&p)
	if errs := s.validateProduct(r.Context(), &p); len(errs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid product", errs)
		return
	}
	if err := s.Store.UpdateProduct(r.Context(), &p); err != nil {
		s.storeError(w, err)
		return
	}
	s.syncReminders(r.Context(), p)
	writeJSON(w, http.StatusOK, s.view(p))
}

func (s *Server) handleDeleteProduct(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	id := r.PathValue("id")
	if _, err := s.Store.GetProduct(r.Context(), u.ID, id); err != nil {
		s.storeError(w, err)
		return
	}
	s.deleteProductFiles(r.Context(), u.ID, id)
	if err := s.Store.DeleteProduct(r.Context(), u.ID, id); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteProductFiles(ctx context.Context, userID, productID string) {
	if s.Files == nil {
		return
	}
	docs, err := s.Store.ListDocuments(ctx, userID, productID)
	if err != nil {
		return
	}
	for _, d := range docs {
		if err := s.Files.Delete(ctx, d.FileKey); err != nil {
			s.Logger.Warn("file delete failed", "key", d.FileKey, "err", err)
		}
	}
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := s.Store.ListCategories(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": cats})
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	ns, err := s.Store.ListNotifications(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": ns})
}
