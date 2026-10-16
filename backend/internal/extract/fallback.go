package extract

import (
	"context"
	"errors"
	"log/slog"
)

// Fallback tries Primary first and falls back to Secondary (rules) when the primary fails for
// any reason (no key, invalid key, quota, timeout, bad JSON). If the primary succeeded but left
// fields empty and text is available, the rule parser fills the gaps.
type Fallback struct {
	Primary   ReceiptExtractor
	Secondary ReceiptExtractor
	Logger    *slog.Logger
}

// Name implements ReceiptExtractor.
func (f *Fallback) Name() string { return "fallback" }

// Extract implements ReceiptExtractor.
func (f *Fallback) Extract(ctx context.Context, in Input) (Result, error) {
	var warnings []string
	if f.Primary != nil {
		res, err := f.Primary.Extract(ctx, in)
		if err == nil {
			if in.Text != "" && len(res.Missing) > 0 && f.Secondary != nil {
				if sec, err2 := f.Secondary.Extract(ctx, in); err2 == nil {
					res.Fields, res.Missing = Clean(merge(res.Fields, sec.Fields))
				}
			}
			return res, nil
		}
		if !errors.Is(err, ErrNotConfigured) {
			warnings = append(warnings, f.Primary.Name()+" unavailable: "+err.Error())
			if f.Logger != nil {
				f.Logger.Warn("primary extractor failed, falling back", "provider", f.Primary.Name(), "err", err)
			}
		}
		if errors.Is(err, ErrEmptyInput) {
			return Result{}, err
		}
	}
	if f.Secondary == nil {
		return Result{}, ErrUnavailable
	}
	res, err := f.Secondary.Extract(ctx, in)
	if err != nil {
		if errors.Is(err, ErrEmptyInput) && len(in.Image) > 0 {
			// image-only and no AI available: user fills the form manually
			_, missing := Clean(Fields{})
			return Result{Provider: "none", Missing: missing, Warnings: append(warnings, "no OCR text available; please fill in manually")}, nil
		}
		return Result{}, err
	}
	res.Warnings = append(warnings, res.Warnings...)
	return res, nil
}

func merge(a, b Fields) Fields {
	pick := func(x, y string) string {
		if x != "" {
			return x
		}
		return y
	}
	a.ProductName = pick(a.ProductName, b.ProductName)
	a.Brand = pick(a.Brand, b.Brand)
	a.Model = pick(a.Model, b.Model)
	a.Seller = pick(a.Seller, b.Seller)
	a.PurchaseDate = pick(a.PurchaseDate, b.PurchaseDate)
	a.SerialNumber = pick(a.SerialNumber, b.SerialNumber)
	if a.Category == "" || a.Category == "other" {
		a.Category = pick(b.Category, a.Category)
	}
	if a.Price == 0 {
		a.Price = b.Price
	}
	if a.WarrantyMonths == 0 {
		a.WarrantyMonths = b.WarrantyMonths
	}
	return a
}
