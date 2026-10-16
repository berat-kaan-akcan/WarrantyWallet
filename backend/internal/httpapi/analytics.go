package httpapi

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

type monthCount struct {
	Month string  `json:"month"` // yyyy-MM
	Count int     `json:"count"`
	Total float64 `json:"total,omitempty"`
}

type brandCount struct {
	Brand string `json:"brand"`
	Count int    `json:"count"`
}

type serviceStats struct {
	Total             int      `json:"total"`
	Active            int      `json:"active"`
	Resolved          int      `json:"resolved"`
	UnderWarranty     int      `json:"under_warranty"`
	TotalCost         float64  `json:"total_cost"`
	AvgResolutionDays *float64 `json:"avg_resolution_days"`
}

// analytics holds the V2 dashboard additions.
type analytics struct {
	PrimaryCurrency  string             `json:"primary_currency"`
	ProtectedValue   map[string]float64 `json:"protected_value"` // price of products still under warranty, per currency
	CoverageRate     float64            `json:"coverage_rate"`   // share of products under warranty (0–1)
	ExpiryTimeline   []monthCount       `json:"expiry_timeline"` // warranties ending in each of the next 12 months
	MonthlyPurchases []monthCount       `json:"monthly_purchases"`
	TopBrands        []brandCount       `json:"top_brands"`
	Service          serviceStats       `json:"service"`
}

func monthKey(t time.Time) string { return t.Format("2006-01") }

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// buildAnalytics derives trends from the user's products and service records.
// Purchase totals only include the primary currency (the one with the most spending).
func buildAnalytics(products []domain.Product, recs []domain.ServiceRecord, today domain.Date) analytics {
	a := analytics{ProtectedValue: map[string]float64{}, TopBrands: []brandCount{}}
	spent := map[string]float64{}
	for _, p := range products {
		spent[p.Currency] += p.Price
	}
	a.PrimaryCurrency = "TRY"
	best := -1.0
	for c, v := range spent {
		if v > best || (v == best && c < a.PrimaryCurrency) {
			a.PrimaryCurrency, best = c, v
		}
	}

	t := today.Time()
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	future := map[string]*monthCount{}
	past := map[string]*monthCount{}
	for i := 0; i < 12; i++ {
		f := start.AddDate(0, i, 0)
		a.ExpiryTimeline = append(a.ExpiryTimeline, monthCount{Month: monthKey(f)})
		p := start.AddDate(0, i-11, 0)
		a.MonthlyPurchases = append(a.MonthlyPurchases, monthCount{Month: monthKey(p)})
	}
	for i := range a.ExpiryTimeline {
		future[a.ExpiryTimeline[i].Month] = &a.ExpiryTimeline[i]
		past[a.MonthlyPurchases[i].Month] = &a.MonthlyPurchases[i]
	}

	brands := map[string]*brandCount{}
	covered := 0
	for _, p := range products {
		if domain.IsCovered(p.WarrantyEnd, today) {
			covered++
			a.ProtectedValue[p.Currency] = round2(a.ProtectedValue[p.Currency] + p.Price)
			if m, ok := future[monthKey(p.WarrantyEnd.Time())]; ok {
				m.Count++
			}
		}
		if m, ok := past[monthKey(p.PurchaseDate.Time())]; ok && !p.PurchaseDate.IsZero() {
			m.Count++
			if p.Currency == a.PrimaryCurrency {
				m.Total = round2(m.Total + p.Price)
			}
		}
		if b := strings.TrimSpace(p.Brand); b != "" {
			k := strings.ToLower(b)
			if brands[k] == nil {
				brands[k] = &brandCount{Brand: b}
			}
			brands[k].Count++
		}
	}
	if len(products) > 0 {
		a.CoverageRate = math.Round(float64(covered)/float64(len(products))*1000) / 1000
	}
	for _, b := range brands {
		a.TopBrands = append(a.TopBrands, *b)
	}
	sort.Slice(a.TopBrands, func(i, j int) bool {
		if a.TopBrands[i].Count == a.TopBrands[j].Count {
			return strings.ToLower(a.TopBrands[i].Brand) < strings.ToLower(a.TopBrands[j].Brand)
		}
		return a.TopBrands[i].Count > a.TopBrands[j].Count
	})
	if len(a.TopBrands) > 5 {
		a.TopBrands = a.TopBrands[:5]
	}

	var days float64
	var n int
	for _, r := range recs {
		a.Service.Total++
		a.Service.TotalCost = round2(a.Service.TotalCost + r.Cost)
		if r.UnderWarranty {
			a.Service.UnderWarranty++
		}
		if !r.Status.Closed() {
			a.Service.Active++
		}
		if r.Status == domain.ServiceResolved {
			a.Service.Resolved++
			if r.ResolvedAt != nil {
				days += math.Floor(r.ResolvedAt.Sub(r.CreatedAt).Hours() / 24)
				n++
			}
		}
	}
	if n > 0 {
		avg := math.Round(days/float64(n)*10) / 10
		a.Service.AvgResolutionDays = &avg
	}
	return a
}
