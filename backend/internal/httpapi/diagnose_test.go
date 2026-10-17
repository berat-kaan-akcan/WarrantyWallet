package httpapi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/assist"
)

type fakeGenerator struct {
	raw string
	err error
}

func (f fakeGenerator) GenerateJSON(context.Context, string, map[string]any) (string, string, error) {
	return f.raw, "gemini-fake", f.err
}

func TestDiagnose(t *testing.T) {
	h := newHarness(t)
	tok := h.register("diag@example.com", "D")
	other := h.register("diag2@example.com", "O")
	p := h.do("POST", "/api/v1/products", tok, map[string]any{"name": "LG OLED TV", "category": "tv", "purchase_date": "2026-01-10"}).json(t)
	path := "/api/v1/products/" + p["id"].(string) + "/diagnose"

	// Rules only (no Gemini configured).
	r := h.do("POST", path, tok, map[string]any{"problem": "screen_lines", "description": "Ekranın ortasında dikey çizgi var"})
	if r.Status != 200 {
		t.Fatalf("rules: %d %s", r.Status, r.Body)
	}
	got := r.json(t)
	if got["provider"] != "rules" || got["recommend_service"] != true || !strings.Contains(got["warranty_advice"].(string), "garanti kapsamında") {
		t.Fatalf("rules body: %v", got)
	}

	// Gemini mock success.
	h.api.Assistant = assist.Assistant{Gemini: fakeGenerator{raw: `{"summary":"Panel kablosu gevşemiş olabilir.","likely_causes":[{"cause":"Panel kablosu","likelihood":"high"}],"steps":[{"title":"Yeniden başlatın"}],"recommend_service":true}`}}
	got = h.do("POST", path, tok, map[string]any{"problem": "screen_lines"}).json(t)
	if got["provider"] != "gemini" || got["model"] != "gemini-fake" || got["warnings"] != nil {
		t.Fatalf("gemini body: %v", got)
	}

	// Gemini failure → rules, internal error is not leaked.
	h.api.Assistant = assist.Assistant{Gemini: fakeGenerator{err: errors.New("secret upstream detail")}}
	r = h.do("POST", path, tok, map[string]any{"problem": "wont_turn_on", "description": "şarjdayken duman çıktı"})
	got = r.json(t)
	if got["provider"] != "rules" || strings.Contains(string(r.Body), "secret upstream") || got["safety_warning"] == nil {
		t.Fatalf("fallback body: %s", r.Body)
	}

	for name, c := range map[string]struct {
		tok  string
		path string
		body map[string]any
		want int
	}{
		"bad problem":     {tok, path, map[string]any{"problem": "exploded"}, 422},
		"empty other":     {tok, path, map[string]any{"problem": "other", "description": "x"}, 422},
		"too long":        {tok, path, map[string]any{"problem": "other", "description": strings.Repeat("a", 2001)}, 422},
		"other's product": {other, path, map[string]any{"problem": "wont_turn_on"}, 404},
		"no auth":         {"", path, map[string]any{"problem": "wont_turn_on"}, 401},
	} {
		if r := h.do("POST", c.path, c.tok, c.body); r.Status != c.want {
			t.Errorf("%s: got %d want %d (%s)", name, r.Status, c.want, r.Body)
		}
	}
}

func TestAdminAIStats(t *testing.T) {
	h := newHarness(t)
	admin := h.register("admin@ww.test", "A")
	tok := h.register("ai@example.com", "U")
	p := h.do("POST", "/api/v1/products", tok, map[string]any{"name": "TV", "purchase_date": "2026-01-10"}).json(t)
	path := "/api/v1/products/" + p["id"].(string) + "/diagnose"
	h.api.Assistant = assist.Assistant{Gemini: fakeGenerator{raw: `{"summary":"s","likely_causes":[{"cause":"c","likelihood":"high"}],"steps":[{"title":"t"}],"recommend_service":false}`}}
	h.do("POST", path, tok, map[string]any{"problem": "wont_turn_on", "description": "gizli açıklama"})
	h.api.Assistant = assist.Assistant{Gemini: fakeGenerator{err: errors.New("503")}}
	h.do("POST", path, tok, map[string]any{"problem": "wont_turn_on"})
	h.api.Assistant = assist.Assistant{}
	h.do("POST", path, tok, map[string]any{"problem": "wont_turn_on"})

	st := h.do("GET", "/api/v1/admin/stats", admin, nil).json(t)
	d := st["diagnoses"].(map[string]any)
	if d["total"].(float64) != 3 || d["ai"].(float64) != 1 || d["rules"].(float64) != 2 || d["ai_failed"].(float64) != 1 || d["by_model"].(map[string]any)["gemini-fake"].(float64) != 1 {
		t.Fatalf("diagnoses %v", d)
	}
	if st["extractions"].(map[string]any)["sample_size"].(float64) != 0 {
		t.Fatalf("diagnoses counted as receipts: %v", st["extractions"])
	}
	r := h.do("GET", "/api/v1/admin/extraction-logs?kind=diagnosis", admin, nil)
	if items := r.json(t)["items"].([]any); len(items) != 3 || strings.Contains(string(r.Body), "gizli") {
		t.Fatalf("logs %s", r.Body)
	}
	if r := h.do("GET", "/api/v1/admin/extraction-logs?kind=x", admin, nil); r.Status != 422 {
		t.Fatalf("bad kind %d", r.Status)
	}
}
