package extract

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

// GeminiExtractor calls the Gemini generateContent REST API with a JSON response schema.
// The API key is sent ONLY in the x-goog-api-key header (never in URLs or logs).
type GeminiExtractor struct {
	APIKey     string
	Model      string   // e.g. gemini-3.8-flash
	Fallbacks  []string // models tried in order if Model is not found (404)
	BaseURL    string   // https://generativelanguage.googleapis.com/v1beta
	HTTPClient *http.Client
	MaxRetries int           // retries for 429/5xx/timeouts (default 2)
	Backoff    time.Duration // base backoff (default 1s)
	Today      func() time.Time
}

// Name implements ReceiptExtractor.
func (g *GeminiExtractor) Name() string { return "gemini" }

const geminiPrompt = `You are an assistant that reads Turkish (or English) purchase receipts, e-invoices (e-Arşiv fatura) and warranty cards.
Extract the PURCHASED PRODUCT (the main durable item, not bags/cables/services) and return ONLY JSON matching the schema.
Rules:
- product_name: brand + model as a human would say it, e.g. "ASUS TUF VG27AQ". No quantities or prices.
- category: one of %s.
- seller: store/merchant name as commonly known (e.g. "MediaMarkt", "Teknosa", "Hepsiburada").
- purchase_date: ISO YYYY-MM-DD. Turkish dates are DD.MM.YYYY. Use null if absent.
- price: total paid for the product as a number (Turkish "12.499,00" means 12499.00). Use null if absent.
- currency: ISO 4217 (TL/₺ → TRY).
- warranty_months: only if the document states the warranty period ("2 yıl" → 24). Otherwise null — do NOT guess.
- serial_number: S/N, seri no or IMEI if printed, else null.
Never invent values that are not supported by the document. Today is %s.`

func (g *GeminiExtractor) schema() map[string]any {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "STRING", "nullable": true, "description": desc}
	}
	return map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"product_name":    str("Brand and model of the purchased product"),
			"brand":           str("Manufacturer brand"),
			"model":           str("Model code, e.g. VG27AQ"),
			"category":        map[string]any{"type": "STRING", "enum": Categories},
			"seller":          str("Store / merchant"),
			"purchase_date":   str("YYYY-MM-DD"),
			"price":           map[string]any{"type": "NUMBER", "nullable": true},
			"currency":        str("ISO 4217 code"),
			"warranty_months": map[string]any{"type": "INTEGER", "nullable": true},
			"serial_number":   str("Serial number or IMEI"),
		},
		"required":         []string{"product_name", "category"},
		"propertyOrdering": []string{"product_name", "brand", "model", "category", "seller", "purchase_date", "price", "currency", "warranty_months", "serial_number"},
	}
}

type gemPart struct {
	Text       string         `json:"text,omitempty"`
	InlineData *gemInlineData `json:"inline_data,omitempty"`
}

type gemInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type gemResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// gemFields mirrors the schema with nullable pointers.
type gemFields struct {
	ProductName    *string  `json:"product_name"`
	Brand          *string  `json:"brand"`
	Model          *string  `json:"model"`
	Category       *string  `json:"category"`
	Seller         *string  `json:"seller"`
	PurchaseDate   *string  `json:"purchase_date"`
	Price          *float64 `json:"price"`
	Currency       *string  `json:"currency"`
	WarrantyMonths *float64 `json:"warranty_months"`
	SerialNumber   *string  `json:"serial_number"`
}

// Extract implements ReceiptExtractor.
func (g *GeminiExtractor) Extract(ctx context.Context, in Input) (Result, error) {
	if strings.TrimSpace(g.APIKey) == "" {
		return Result{}, ErrNotConfigured
	}
	if strings.TrimSpace(in.Text) == "" && len(in.Image) == 0 {
		return Result{}, ErrEmptyInput
	}
	today := time.Now()
	if g.Today != nil {
		today = g.Today()
	}
	parts := []gemPart{{Text: fmt.Sprintf(geminiPrompt, strings.Join(Categories, ", "), today.Format("2006-01-02"))}}
	if len(in.Image) > 0 {
		mime := in.MIME
		if mime == "" {
			mime = http.DetectContentType(in.Image)
		}
		parts = append(parts, gemPart{InlineData: &gemInlineData{MimeType: mime, Data: base64.StdEncoding.EncodeToString(in.Image)}})
	}
	if t := strings.TrimSpace(in.Text); t != "" {
		if len(t) > 20000 {
			t = t[:20000]
		}
		parts = append(parts, gemPart{Text: "OCR text of the document (may contain errors):\n" + t})
	}
	body, err := json.Marshal(map[string]any{
		"contents": []any{map[string]any{"role": "user", "parts": parts}},
		"generationConfig": map[string]any{
			"temperature":      0,
			"responseMimeType": "application/json",
			"responseSchema":   g.schema(),
		},
	})
	if err != nil {
		return Result{}, err
	}

	models := append([]string{g.Model}, g.Fallbacks...)
	var lastErr error
	for _, model := range models {
		if model == "" {
			continue
		}
		raw, err := g.callWithRetry(ctx, model, body)
		if err == errModelNotFound {
			lastErr = fmt.Errorf("%w: model %s not found", ErrUnavailable, model)
			continue
		}
		if err != nil {
			return Result{}, err
		}
		f, err := parseGeminiJSON(raw)
		if err != nil {
			return Result{}, err
		}
		f, missing := Clean(f)
		return Result{Fields: f, Provider: "gemini", RawText: strings.TrimSpace(in.Text), Missing: missing}, nil
	}
	if lastErr == nil {
		lastErr = ErrNotConfigured
	}
	return Result{}, lastErr
}

var errModelNotFound = fmt.Errorf("model not found")

func (g *GeminiExtractor) callWithRetry(ctx context.Context, model string, body []byte) (string, error) {
	retries := g.MaxRetries
	if retries == 0 {
		retries = 2
	}
	backoff := g.Backoff
	if backoff == 0 {
		backoff = time.Second
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			wait := backoff*time.Duration(1<<(attempt-1)) + time.Duration(rand.Int64N(int64(backoff/2+1)))
			select {
			case <-ctx.Done():
				return "", fmt.Errorf("%w: %v", ErrUnavailable, ctx.Err())
			case <-time.After(wait):
			}
		}
		text, retry, err := g.call(ctx, model, body)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if !retry {
			return "", err
		}
	}
	return "", lastErr
}

// call performs one request. retry reports whether the error is transient.
func (g *GeminiExtractor) call(ctx context.Context, model string, body []byte) (string, bool, error) {
	base := strings.TrimRight(g.BaseURL, "/")
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}
	url := fmt.Sprintf("%s/models/%s:generateContent", base, model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.APIKey)
	client := g.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", false, fmt.Errorf("%w: %v", ErrUnavailable, ctx.Err())
		}
		return "", true, fmt.Errorf("%w: %s", ErrUnavailable, redact(err.Error(), g.APIKey))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var gr gemResponse
	_ = json.Unmarshal(data, &gr)
	msg := ""
	if gr.Error != nil {
		msg = gr.Error.Status + " " + gr.Error.Message
	}
	msg = redact(msg, g.APIKey)
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", false, errModelNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", true, fmt.Errorf("%w: %s", ErrQuota, msg)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", false, fmt.Errorf("%w: %s", ErrAuth, msg)
	case resp.StatusCode == http.StatusBadRequest && strings.Contains(strings.ToLower(msg), "api key"):
		return "", false, fmt.Errorf("%w: %s", ErrAuth, msg)
	case resp.StatusCode >= 500:
		return "", true, fmt.Errorf("%w: HTTP %d %s", ErrUnavailable, resp.StatusCode, msg)
	case resp.StatusCode != http.StatusOK:
		return "", false, fmt.Errorf("%w: HTTP %d %s", ErrUnavailable, resp.StatusCode, msg)
	}
	if gr.PromptFeedback != nil && gr.PromptFeedback.BlockReason != "" {
		return "", false, fmt.Errorf("%w: blocked (%s)", ErrBadResponse, gr.PromptFeedback.BlockReason)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return "", false, fmt.Errorf("%w: no candidates", ErrBadResponse)
	}
	var sb strings.Builder
	for _, p := range gr.Candidates[0].Content.Parts {
		sb.WriteString(p.Text)
	}
	return sb.String(), false, nil
}

func parseGeminiJSON(raw string) (Fields, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	var gf gemFields
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &gf); err != nil {
		return Fields{}, fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	s := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	f := Fields{
		ProductName: s(gf.ProductName), Brand: s(gf.Brand), Model: s(gf.Model), Category: s(gf.Category),
		Seller: s(gf.Seller), PurchaseDate: s(gf.PurchaseDate), Currency: s(gf.Currency), SerialNumber: s(gf.SerialNumber),
	}
	if gf.Price != nil {
		f.Price = *gf.Price
	}
	if gf.WarrantyMonths != nil {
		f.WarrantyMonths = int(*gf.WarrantyMonths)
	}
	return f, nil
}

func redact(s, key string) string {
	if key != "" {
		s = strings.ReplaceAll(s, key, "[REDACTED]")
	}
	return s
}
