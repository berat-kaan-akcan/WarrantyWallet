package httpapi

import "testing"

func TestServiceHistory(t *testing.T) {
	h := newHarness(t)
	tok := h.register("hist@example.com", "H")
	other := h.register("hist2@example.com", "O")
	p := h.do("POST", "/api/v1/products", tok, map[string]any{"name": "Arçelik Buzdolabı", "purchase_date": "2026-01-10"}).json(t)
	pid := p["id"].(string)

	rec := h.do("POST", "/api/v1/products/"+pid+"/service-records", tok, map[string]any{"problem": "wont_turn_on", "event_note": "Kayıt açıldı"}).json(t)
	id := rec["id"].(string)
	path := "/api/v1/service-records/" + id
	if rec["resolved_at"] != nil || rec["cost"].(float64) != 0 {
		t.Fatalf("new record %v", rec)
	}

	if r := h.do("PATCH", path, tok, map[string]any{"status": "in_service", "service_name": "Yetkili Servis", "event_note": "Servise teslim edildi"}); r.Status != 200 {
		t.Fatalf("patch %d %s", r.Status, r.Body)
	}
	h.do("PATCH", path, tok, map[string]any{"notes": "fiş verildi"}) // no status change → no event
	got := h.do("PATCH", path, tok, map[string]any{"status": "resolved", "cost": 450.75}).json(t)
	if got["resolved_at"] == nil || got["cost"].(float64) != 450.75 {
		t.Fatalf("resolved %v", got)
	}

	detail := h.do("GET", path, tok, nil).json(t)
	events := detail["events"].([]any)
	if len(events) != 3 || detail["product_name"] != "Arçelik Buzdolabı" {
		t.Fatalf("detail %v", detail)
	}
	if e := events[1].(map[string]any); e["status"] != "in_service" || e["note"] != "Servise teslim edildi" {
		t.Fatalf("event %v", e)
	}
	if e := events[0].(map[string]any); e["status"] != "open" || e["note"] != "Kayıt açıldı" {
		t.Fatalf("first event %v", e)
	}

	// Reopening clears resolved_at.
	if got := h.do("PATCH", path, tok, map[string]any{"status": "open"}).json(t); got["resolved_at"] != nil {
		t.Fatalf("reopen %v", got)
	}
	h.do("PATCH", path, tok, map[string]any{"status": "resolved"})
	h.do("POST", "/api/v1/products/"+pid+"/service-records", tok, map[string]any{"problem": "other"})

	list := h.do("GET", "/api/v1/service-records?status=resolved", tok, nil).json(t)
	sum := list["summary"].(map[string]any)
	if len(list["items"].([]any)) != 1 || sum["total"].(float64) != 2 || sum["active"].(float64) != 1 || sum["total_cost"].(float64) != 450.75 || sum["avg_resolution_days"] == nil {
		t.Fatalf("list %v", list)
	}
	if by := sum["by_status"].(map[string]any); by["open"].(float64) != 1 || by["resolved"].(float64) != 1 {
		t.Fatalf("by_status %v", by)
	}

	// Ordered checks (map iteration order is random).
	for _, c := range []struct {
		name, method, tok, path string
		body                    map[string]any
		want                    int
	}{
		{"bad status filter", "GET", tok, "/api/v1/service-records?status=done", nil, 422},
		{"negative cost", "PATCH", tok, path, map[string]any{"cost": -1}, 422},
		{"other get", "GET", other, path, nil, 404},
		{"other delete", "DELETE", other, path, nil, 404},
		{"delete", "DELETE", tok, path, nil, 204},
		{"deleted", "GET", tok, path, nil, 404},
	} {
		if r := h.do(c.method, c.path, c.tok, c.body); r.Status != c.want {
			t.Errorf("%s: got %d want %d (%s)", c.name, r.Status, c.want, r.Body)
		}
	}
}
