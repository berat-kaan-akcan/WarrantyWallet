package httpapi

import (
	"strings"
	"testing"
)

func TestInputValidation(t *testing.T) {
	h := newHarness(t)
	tok := h.register("val@example.com", "V")
	if r := h.do("GET", "/api/v1/products?status=bogus", tok, nil); r.Status != 422 {
		t.Fatalf("bad status filter %d", r.Status)
	}
	if r := h.do("GET", "/api/v1/products?q="+strings.Repeat("a", 101), tok, nil); r.Status != 422 {
		t.Fatalf("long query %d", r.Status)
	}
	r := h.do("POST", "/api/v1/products", tok, map[string]any{"name": "TV", "purchase_date": "1985-01-01", "price": 2e9, "currency": "tl"})
	if r.Status != 422 {
		t.Fatalf("product validation %d %s", r.Status, r.Body)
	}
	f := r.json(t)["error"].(map[string]any)["fields"].(map[string]any)
	if f["purchase_date"] == nil || f["price"] == nil {
		t.Fatalf("fields %v", f)
	}
	p := h.do("POST", "/api/v1/products", tok, map[string]any{"name": "TV", "purchase_date": "2026-01-10", "warranty_months": 24})
	if p.Status != 201 {
		t.Fatalf("create %d %s", p.Status, p.Body)
	}
	id := p.json(t)["id"].(string)
	if r := h.do("POST", "/api/v1/products/"+id+"/service-records", tok, map[string]any{"problem": "other", "service_date": "2030-01-01"}); r.Status != 422 {
		t.Fatalf("far future service date %d", r.Status)
	}
	if r := h.do("POST", "/api/v1/products/"+id+"/service-records", tok, map[string]any{"problem": "other", "description": strings.Repeat("ş", 2000)}); r.Status != 201 {
		t.Fatalf("2000 Turkish chars must be accepted %d %s", r.Status, r.Body)
	}
}
