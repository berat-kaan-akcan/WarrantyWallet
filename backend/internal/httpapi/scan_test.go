package httpapi

import "testing"

func TestProductLookupByScannedCode(t *testing.T) {
	h := newHarness(t)
	tok := h.register("scan@example.com", "S")
	other := h.register("scan2@example.com", "O")
	body := map[string]any{"name": "Sony WH-1000XM5", "purchase_date": "2026-10-03", "serial_number": "5023417", "barcode": "4548736132610"}
	r := h.do("POST", "/api/v1/products", tok, body)
	if r.Status != 201 || r.json(t)["barcode"] != "4548736132610" {
		t.Fatalf("create %d %s", r.Status, r.Body)
	}
	for _, code := range []string{"4548736132610", "5023417"} {
		got := h.do("GET", "/api/v1/products?code="+code, tok, nil).json(t)
		if got["count"].(float64) != 1 {
			t.Fatalf("lookup %s: %v", code, got)
		}
	}
	if got := h.do("GET", "/api/v1/products?code=4548736132610", other, nil).json(t); got["count"].(float64) != 0 {
		t.Fatal("other user's product must not be found by code")
	}
}
