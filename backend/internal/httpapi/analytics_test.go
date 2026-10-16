package httpapi

import (
	"testing"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

func TestBuildAnalytics(t *testing.T) {
	today := domain.NewDate(2026, 10, 16)
	d := domain.NewDate
	products := []domain.Product{
		{Brand: "Sony", Price: 12000, Currency: "TRY", PurchaseDate: d(2026, 10, 3), WarrantyEnd: d(2028, 10, 3)},
		{Brand: "sony ", Price: 3000, Currency: "TRY", PurchaseDate: d(2025, 11, 20), WarrantyEnd: d(2026, 11, 20)},
		{Brand: "ASUS", Price: 500, Currency: "EUR", PurchaseDate: d(2026, 9, 1), WarrantyEnd: d(2027, 3, 1)},
		{Brand: "LG", Price: 8000, Currency: "TRY", PurchaseDate: d(2023, 1, 1), WarrantyEnd: d(2025, 1, 1)}, // expired, too old
		{Price: 100, Currency: "TRY"}, // unknown dates
	}
	resolved := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	recs := []domain.ServiceRecord{
		{Status: domain.ServiceResolved, Cost: 450.5, UnderWarranty: false, CreatedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), ResolvedAt: &resolved},
		{Status: domain.ServiceInService, UnderWarranty: true},
		{Status: domain.ServiceCancelled, UnderWarranty: true},
	}
	a := buildAnalytics(products, recs, today)

	if a.PrimaryCurrency != "TRY" || a.ProtectedValue["TRY"] != 15000 || a.ProtectedValue["EUR"] != 500 {
		t.Fatalf("value %+v", a)
	}
	if a.CoverageRate != 0.6 {
		t.Errorf("coverage %v", a.CoverageRate)
	}
	if len(a.ExpiryTimeline) != 12 || a.ExpiryTimeline[0].Month != "2026-10" || a.ExpiryTimeline[1].Count != 1 || a.ExpiryTimeline[5].Count != 1 {
		t.Errorf("expiry %+v", a.ExpiryTimeline)
	}
	mp := a.MonthlyPurchases
	if len(mp) != 12 || mp[0].Month != "2025-11" || mp[11].Month != "2026-10" {
		t.Fatalf("months %+v", mp)
	}
	if mp[0].Count != 1 || mp[0].Total != 3000 || mp[11].Total != 12000 || mp[10].Count != 1 || mp[10].Total != 0 {
		t.Errorf("purchases %+v", mp) // EUR purchase counted but not summed
	}
	if len(a.TopBrands) != 3 || a.TopBrands[0].Brand != "Sony" || a.TopBrands[0].Count != 2 || a.TopBrands[1].Brand != "ASUS" {
		t.Errorf("brands %+v", a.TopBrands)
	}
	s := a.Service
	if s.Total != 3 || s.Active != 1 || s.Resolved != 1 || s.UnderWarranty != 2 || s.TotalCost != 450.5 || s.AvgResolutionDays == nil || *s.AvgResolutionDays != 10 {
		t.Errorf("service %+v", s)
	}

	empty := buildAnalytics(nil, nil, today)
	if empty.CoverageRate != 0 || len(empty.TopBrands) != 0 || empty.Service.AvgResolutionDays != nil || empty.PrimaryCurrency != "TRY" {
		t.Errorf("empty %+v", empty)
	}
}

func TestDashboardIncludesAnalytics(t *testing.T) {
	h := newHarness(t)
	tok := h.register("an@example.com", "A")
	h.do("POST", "/api/v1/products", tok, map[string]any{"name": "TV", "brand": "LG", "price": 20000, "purchase_date": "2026-01-10"})
	d := h.do("GET", "/api/v1/dashboard", tok, nil).json(t)
	a, ok := d["analytics"].(map[string]any)
	if !ok || a["coverage_rate"].(float64) != 1 || len(a["expiry_timeline"].([]any)) != 12 || a["primary_currency"] != "TRY" {
		t.Fatalf("analytics %v", d["analytics"])
	}
}
