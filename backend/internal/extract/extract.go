// Package extract turns a receipt/invoice (image and/or OCR text) into structured product data.
//
// ReceiptExtractor is the swappable seam: GeminiExtractor (LLM), RuleExtractor (regex/heuristics)
// and Fallback (Gemini first, rules on any failure). The user always reviews and can correct
// every field before saving, so extraction is a suggestion, never the source of truth.
package extract

import (
	"context"
	"errors"
	"strings"
)

// Input to an extractor. At least one of Text or Image must be set.
type Input struct {
	Text  string // raw OCR text (e.g. from on-device ML Kit)
	Image []byte // receipt photo / PDF bytes
	MIME  string // image/jpeg, image/png, image/webp, image/heic, application/pdf
}

// Fields extracted from a receipt. Empty/zero means "not found".
type Fields struct {
	ProductName    string  `json:"product_name"`
	Brand          string  `json:"brand"`
	Model          string  `json:"model"`
	Category       string  `json:"category"`
	Seller         string  `json:"seller"`
	PurchaseDate   string  `json:"purchase_date"` // YYYY-MM-DD
	Price          float64 `json:"price"`
	Currency       string  `json:"currency"`
	WarrantyMonths int     `json:"warranty_months"`
	SerialNumber   string  `json:"serial_number"`
}

// Result of an extraction.
type Result struct {
	Fields   Fields   `json:"fields"`
	Provider string   `json:"provider"` // "gemini" | "rules"
	RawText  string   `json:"raw_text,omitempty"`
	Missing  []string `json:"missing"`            // fields the user must fill in
	Warnings []string `json:"warnings,omitempty"` // e.g. "gemini unavailable: quota exceeded"
}

// ReceiptExtractor extracts structured fields from a receipt.
type ReceiptExtractor interface {
	Name() string
	Extract(ctx context.Context, in Input) (Result, error)
}

// Errors.
var (
	ErrEmptyInput    = errors.New("no text or image provided")
	ErrNotConfigured = errors.New("extractor not configured")
	ErrQuota         = errors.New("quota exceeded / rate limited")
	ErrAuth          = errors.New("api key rejected")
	ErrBadResponse   = errors.New("unparseable model response")
	ErrUnavailable   = errors.New("provider unavailable")
)

// Categories the extractor may return (must match domain.DefaultCategories slugs).
var Categories = []string{"phone", "laptop", "monitor", "tv", "headphones", "tablet", "appliance",
	"small_appliance", "camera", "console", "wearable", "other"}

func validCategory(c string) bool {
	for _, x := range Categories {
		if x == c {
			return true
		}
	}
	return false
}

// Clean normalises fields and computes the missing list.
func Clean(f Fields) (Fields, []string) {
	f.ProductName = strings.TrimSpace(f.ProductName)
	f.Brand = strings.TrimSpace(f.Brand)
	f.Model = strings.TrimSpace(f.Model)
	f.Seller = strings.TrimSpace(f.Seller)
	f.SerialNumber = strings.TrimSpace(f.SerialNumber)
	f.Category = strings.ToLower(strings.TrimSpace(f.Category))
	if !validCategory(f.Category) {
		if g := GuessCategory(f.ProductName + " " + f.Model); g != "" {
			f.Category = g
		} else {
			f.Category = "other"
		}
	}
	f.Currency = strings.ToUpper(strings.TrimSpace(f.Currency))
	switch f.Currency {
	case "TL", "₺", "YTL":
		f.Currency = "TRY"
	case "$":
		f.Currency = "USD"
	case "€":
		f.Currency = "EUR"
	}
	if len(f.Currency) != 3 {
		f.Currency = "TRY"
	}
	if f.Price < 0 {
		f.Price = 0
	}
	if f.WarrantyMonths < 0 || f.WarrantyMonths > 240 {
		f.WarrantyMonths = 0
	}
	if f.PurchaseDate != "" {
		if d, ok := parseAnyDate(f.PurchaseDate); ok {
			f.PurchaseDate = d
		} else {
			f.PurchaseDate = ""
		}
	}
	var missing []string
	if f.ProductName == "" {
		missing = append(missing, "product_name")
	}
	if f.PurchaseDate == "" {
		missing = append(missing, "purchase_date")
	}
	if f.Price == 0 {
		missing = append(missing, "price")
	}
	if f.WarrantyMonths == 0 {
		missing = append(missing, "warranty_months")
	}
	if missing == nil {
		missing = []string{}
	}
	return f, missing
}
