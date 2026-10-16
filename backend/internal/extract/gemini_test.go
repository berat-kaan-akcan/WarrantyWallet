package extract

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fakeKey = "test-key-not-real"

func geminiOK(jsonText string) string {
	b, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{
		"content": map[string]any{"parts": []any{map[string]any{"text": jsonText}}}, "finishReason": "STOP"}}})
	return string(b)
}

func newGemini(url string) *GeminiExtractor {
	return &GeminiExtractor{APIKey: fakeKey, Model: "gemini-test", BaseURL: url, Backoff: time.Millisecond,
		HTTPClient: &http.Client{Timeout: 2 * time.Second}, Today: func() time.Time { return time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC) }}
}

func TestGeminiSuccessRequestShape(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-test:generateContent" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != fakeKey {
			t.Errorf("api key header missing")
		}
		if strings.Contains(r.URL.RawQuery, fakeKey) {
			t.Errorf("api key leaked into URL")
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = io.WriteString(w, geminiOK(`{"product_name":"ASUS TUF VG27AQ","brand":"ASUS","model":"VG27AQ","category":"monitor","seller":"MediaMarkt","purchase_date":"2026-09-12","price":12499,"currency":"TRY","warranty_months":24,"serial_number":null}`))
	}))
	defer srv.Close()

	res, err := newGemini(srv.URL).Extract(context.Background(), Input{Text: receiptMediaMarkt, Image: []byte("\xff\xd8\xff fake jpeg"), MIME: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	f := res.Fields
	if res.Provider != "gemini" || f.ProductName != "ASUS TUF VG27AQ" || f.Price != 12499 || f.WarrantyMonths != 24 || f.PurchaseDate != "2026-09-12" || f.Category != "monitor" {
		t.Fatalf("result %+v", res)
	}
	gc := gotBody["generationConfig"].(map[string]any)
	if gc["responseMimeType"] != "application/json" || gc["responseSchema"] == nil {
		t.Fatalf("structured output not requested: %v", gc)
	}
	parts := gotBody["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	if len(parts) != 3 || parts[1].(map[string]any)["inline_data"] == nil {
		t.Fatalf("expected prompt + image + text parts, got %v", parts)
	}
}

func TestGeminiRetriesThenQuota(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"quota"}}`)
	}))
	defer srv.Close()
	_, err := newGemini(srv.URL).Extract(context.Background(), Input{Text: "x"})
	if !errors.Is(err, ErrQuota) {
		t.Fatalf("want quota err, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 1 call + 2 retries, got %d", calls)
	}
}

func TestGeminiRetryRecovers(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, geminiOK(`{"product_name":"Sony WH-1000XM5","category":"headphones"}`))
	}))
	defer srv.Close()
	res, err := newGemini(srv.URL).Extract(context.Background(), Input{Text: "x"})
	if err != nil || res.Fields.ProductName != "Sony WH-1000XM5" {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestGeminiAuthErrorNoRetryAndRedacted(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":401,"status":"UNAUTHENTICATED","message":"bad key `+fakeKey+`"}}`)
	}))
	defer srv.Close()
	_, err := newGemini(srv.URL).Extract(context.Background(), Input{Text: "x"})
	if !errors.Is(err, ErrAuth) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	if strings.Contains(err.Error(), fakeKey) {
		t.Fatal("api key leaked in error message")
	}
}

func TestGeminiModelFallbackOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "gemini-test") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, geminiOK(`{"product_name":"X","category":"other"}`))
	}))
	defer srv.Close()
	g := newGemini(srv.URL)
	g.Fallbacks = []string{"gemini-other"}
	if _, err := g.Extract(context.Background(), Input{Text: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestGeminiBadJSONAndTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, geminiOK(`not json`))
	}))
	defer srv.Close()
	if _, err := newGemini(srv.URL).Extract(context.Background(), Input{Text: "x"}); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want bad response, got %v", err)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer slow.Close()
	g := newGemini(slow.URL)
	g.HTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	g.MaxRetries = 1
	if _, err := g.Extract(context.Background(), Input{Text: "x"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want unavailable on timeout, got %v", err)
	}
}

func TestGeminiNotConfigured(t *testing.T) {
	if _, err := (&GeminiExtractor{}).Extract(context.Background(), Input{Text: "x"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
}

type stubExtractor struct {
	res Result
	err error
	n   int
}

func (s *stubExtractor) Name() string { return "stub" }
func (s *stubExtractor) Extract(context.Context, Input) (Result, error) {
	s.n++
	return s.res, s.err
}

func TestFallbackUsesRulesOnQuota(t *testing.T) {
	primary := &stubExtractor{err: ErrQuota}
	fb := &Fallback{Primary: primary, Secondary: RuleExtractor{}}
	res, err := fb.Extract(context.Background(), Input{Text: receiptMediaMarkt})
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider != "rules" || res.Fields.Seller != "MediaMarkt" || len(res.Warnings) != 1 {
		t.Fatalf("%+v", res)
	}
}

func TestFallbackNoKeySilent(t *testing.T) {
	fb := &Fallback{Primary: &GeminiExtractor{}, Secondary: RuleExtractor{}}
	res, err := fb.Extract(context.Background(), Input{Text: receiptTeknosa})
	if err != nil || res.Provider != "rules" || len(res.Warnings) != 0 {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestFallbackFillsGapsFromRules(t *testing.T) {
	primary := &stubExtractor{res: Result{Provider: "gemini", Fields: Fields{ProductName: "ASUS TUF VG27AQ", Category: "monitor"}, Missing: []string{"price"}}}
	fb := &Fallback{Primary: primary, Secondary: RuleExtractor{}}
	res, _ := fb.Extract(context.Background(), Input{Text: receiptMediaMarkt})
	if res.Provider != "gemini" || res.Fields.Price != 12499 || res.Fields.ProductName != "ASUS TUF VG27AQ" || len(res.Missing) != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestFallbackImageOnlyNoAI(t *testing.T) {
	fb := &Fallback{Primary: &GeminiExtractor{}, Secondary: RuleExtractor{}}
	res, err := fb.Extract(context.Background(), Input{Image: []byte{0xff, 0xd8}})
	if err != nil || res.Provider != "none" || len(res.Missing) == 0 {
		t.Fatalf("%v %+v", err, res)
	}
}
