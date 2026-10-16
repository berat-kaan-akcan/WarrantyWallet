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
