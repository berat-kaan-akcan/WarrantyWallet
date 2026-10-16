package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func d(s string) Date {
	v, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestWarrantyEnd(t *testing.T) {
	cases := []struct {
		purchase string
		months   int
		want     string
	}{
		{"2026-09-12", 24, "2028-09-12"},
		{"2026-01-31", 1, "2026-02-28"},
		{"2028-01-31", 1, "2028-02-29"}, // leap year
		{"2024-02-29", 12, "2025-02-28"},
		{"2026-12-15", 1, "2027-01-15"},
		{"2026-03-31", 6, "2026-09-30"},
		{"2026-05-10", 36, "2029-05-10"},
		{"2026-05-10", 0, ""},
		{"2026-05-10", -3, ""},
	}
	for _, c := range cases {
		got := WarrantyEnd(d(c.purchase), c.months).String()
		if got != c.want {
			t.Errorf("WarrantyEnd(%s,%d)=%q want %q", c.purchase, c.months, got, c.want)
		}
	}
	if !WarrantyEnd(Date{}, 24).IsZero() {
		t.Error("zero purchase date must yield zero end")
	}
}

func TestAddMonthsNegative(t *testing.T) {
	if got := AddMonths(d("2026-03-31"), -1).String(); got != "2026-02-28" {
		t.Fatalf("got %s", got)
	}
	if got := AddMonths(d("2026-01-15"), -2).String(); got != "2025-11-15" {
		t.Fatalf("got %s", got)
	}
}

func TestStatusOf(t *testing.T) {
	end := d("2026-12-31")
	cases := []struct {
		today string
		want  WarrantyStatus
		left  int
	}{
		{"2026-01-01", StatusActive, 364},
		{"2026-12-01", StatusExpiringSoon, 30},
		{"2026-12-31", StatusExpiringSoon, 0},
		{"2027-01-01", StatusExpired, -1},
	}
	for _, c := range cases {
		if got := StatusOf(end, d(c.today)); got != c.want {
			t.Errorf("StatusOf(%s)=%s want %s", c.today, got, c.want)
		}
		if got := DaysLeft(end, d(c.today)); got != c.left {
			t.Errorf("DaysLeft(%s)=%d want %d", c.today, got, c.left)
		}
	}
	if StatusOf(Date{}, d("2026-01-01")) != StatusUnknown {
		t.Error("zero end should be unknown")
	}
	if !IsCovered(end, d("2026-12-31")) || IsCovered(end, d("2027-01-01")) {
		t.Error("IsCovered wrong")
	}
}

func TestDateJSON(t *testing.T) {
	var v struct {
		D Date `json:"d"`
		E Date `json:"e"`
	}
	if err := json.Unmarshal([]byte(`{"d":"2026-09-12","e":null}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.D.String() != "2026-09-12" || !v.E.IsZero() {
		t.Fatalf("bad decode %+v", v)
	}
	b, _ := json.Marshal(v)
	if string(b) != `{"d":"2026-09-12","e":null}` {
		t.Fatalf("bad encode %s", b)
	}
	if err := json.Unmarshal([]byte(`{"d":"12.09.2026"}`), &v); err == nil {
		t.Fatal("expected error for non-ISO date")
	}
}

func TestDateScan(t *testing.T) {
	var x Date
	if err := x.Scan(time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)); err != nil || x.String() != "2026-09-12" {
		t.Fatalf("scan time: %v %s", err, x)
	}
	if err := x.Scan("2026-01-02"); err != nil || x.String() != "2026-01-02" {
		t.Fatalf("scan string: %v %s", err, x)
	}
	if err := x.Scan(nil); err != nil || !x.IsZero() {
		t.Fatal("scan nil")
	}
	if err := x.Scan(42); err == nil {
		t.Fatal("expected error")
	}
}

func TestProductNormalizeValidate(t *testing.T) {
	p := Product{Name: "  ASUS TUF VG27AQ ", Category: " Monitor ", PurchaseDate: d("2026-09-12"), WarrantyMonths: 24, WarrantyEnd: d("2099-01-01")}
	p.Normalize()
	if p.Name != "ASUS TUF VG27AQ" || p.Category != "monitor" || p.Currency != "TRY" {
		t.Fatalf("normalize: %+v", p)
	}
	if p.WarrantyEnd.String() != "2028-09-12" {
		t.Fatalf("end must be recomputed, got %s", p.WarrantyEnd)
	}
	if errs := p.Validate(d("2026-10-16")); len(errs) != 0 {
		t.Fatalf("unexpected errors %v", errs)
	}
	bad := Product{Price: -1, WarrantyMonths: 999, PurchaseDate: d("2030-01-01"), Currency: "TL"}
	errs := bad.Validate(d("2026-10-16"))
	for _, f := range []string{"name", "price", "warranty_months", "purchase_date", "currency"} {
		if _, ok := errs[f]; !ok {
			t.Errorf("expected error on %s, got %v", f, errs)
		}
	}
}

func TestNewID(t *testing.T) {
	a, b := NewID(), NewID()
	if a == b || len(a) != 36 || a[14] != '4' {
		t.Fatalf("bad ids %s %s", a, b)
	}
}

func TestProductValidateBounds(t *testing.T) {
	today := NewDate(2026, 10, 16)
	base := func() Product {
		p := Product{Name: "Çamaşır Makinesi", PurchaseDate: NewDate(2026, 1, 2), Price: 100, Currency: "TRY", WarrantyMonths: 24}
		p.Normalize()
		return p
	}
	if p := base(); len(p.Validate(today)) != 0 {
		errs := p.Validate(today)
		t.Fatalf("valid product rejected: %v", errs)
	}
	cases := map[string]func(p *Product){
		"price":         func(p *Product) { p.Price = MaxPrice + 1 },
		"currency":      func(p *Product) { p.Currency = "T1Y" },
		"purchase_date": func(p *Product) { p.PurchaseDate = NewDate(1989, 12, 31) },
		"name":          func(p *Product) { p.Name = strings.Repeat("ş", 201) },
		"brand":         func(p *Product) { p.Brand = strings.Repeat("x", 121) },
	}
	for field, mutate := range cases {
		p := base()
		mutate(&p)
		if errs := p.Validate(today); errs[field] == "" {
			t.Errorf("%s: expected error, got %v", field, errs)
		}
	}
	// 200 Turkish characters are 400 bytes but still valid.
	p := base()
	p.Name = strings.Repeat("ğ", 200)
	if errs := p.Validate(today); errs["name"] != "" {
		t.Fatalf("rune count not used: %v", errs)
	}
}
