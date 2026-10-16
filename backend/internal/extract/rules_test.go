package extract

import (
	"context"
	"errors"
	"testing"
)

const receiptMediaMarkt = `MediaMarkt
MMS Istanbul Elektronik Tic. Ltd. Sti.
Tarih: 12.09.2026   Saat: 14:32
FIS NO: 0042
ASUS TUF VG27AQ 27" MONITOR   1 AD   12.499,00
KDV %20                         2.083,17
TOPLAM                      *12.499,00 TL
Urun 24 AY GARANTILIDIR
Seri No: M1LMQS123456
`

const receiptTeknosa = `TEKNOSA IC VE DIS TICARET A.S.
FATURA
Duzenleme Tarihi 03/02/2025
Sony WH-1000XM5 Kablosuz Kulaklik  x1  8.999,90
ARA TOPLAM 8.999,90
GENEL TOPLAM 8.999,90 TL
Garanti Suresi: 2 Yil
`

const receiptCoffee = `Kahve Dunyasi Ev Urunleri
12 Mart 2026
DeLonghi Dedica EC685 Espresso Kahve Makinesi
TUTAR: 6.250,00 TL
`

func TestRuleExtractorMediaMarkt(t *testing.T) {
	res, err := RuleExtractor{}.Extract(context.Background(), Input{Text: receiptMediaMarkt})
	if err != nil {
		t.Fatal(err)
	}
	f := res.Fields
	check(t, "seller", f.Seller, "MediaMarkt")
	check(t, "brand", f.Brand, "ASUS")
	check(t, "date", f.PurchaseDate, "2026-09-12")
	check(t, "category", f.Category, "monitor")
	check(t, "currency", f.Currency, "TRY")
	check(t, "serial", f.SerialNumber, "M1LMQS123456")
	if f.Price != 12499 {
		t.Errorf("price %v", f.Price)
	}
	if f.WarrantyMonths != 24 {
		t.Errorf("warranty %d", f.WarrantyMonths)
	}
	if f.ProductName == "" || f.ProductName[:4] != "ASUS" {
		t.Errorf("product name %q", f.ProductName)
	}
	check(t, "model", f.Model, "VG27AQ")
	if res.Provider != "rules" || len(res.Missing) != 0 {
		t.Errorf("provider=%s missing=%v", res.Provider, res.Missing)
	}
}

func TestRuleExtractorTeknosa(t *testing.T) {
	res, _ := RuleExtractor{}.Extract(context.Background(), Input{Text: receiptTeknosa})
	f := res.Fields
	check(t, "seller", f.Seller, "Teknosa")
	check(t, "brand", f.Brand, "Sony")
	check(t, "date", f.PurchaseDate, "2025-02-03")
	check(t, "category", f.Category, "headphones")
	if f.Price != 8999.90 {
		t.Errorf("price %v", f.Price)
	}
	if f.WarrantyMonths != 24 {
		t.Errorf("warranty (2 yıl) %d", f.WarrantyMonths)
	}
}

func TestRuleExtractorCoffeeMachineMissingWarranty(t *testing.T) {
	res, _ := RuleExtractor{}.Extract(context.Background(), Input{Text: receiptCoffee})
	f := res.Fields
	check(t, "date", f.PurchaseDate, "2026-03-12")
	check(t, "category", f.Category, "small_appliance")
	check(t, "brand", f.Brand, "Delonghi")
	if f.Price != 6250 {
		t.Errorf("price %v", f.Price)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "warranty_months" {
		t.Errorf("missing %v", res.Missing)
	}
}

func TestRuleExtractorEmpty(t *testing.T) {
	if _, err := (RuleExtractor{}).Extract(context.Background(), Input{Image: []byte{1}}); !errors.Is(err, ErrEmptyInput) {
		t.Fatalf("want ErrEmptyInput, got %v", err)
	}
}

func TestParseAmount(t *testing.T) {
	cases := map[string]float64{
		"12.499,00": 12499, "12,499.00": 12499, "12499.90": 12499.9, "1 299,90": 1299.9,
		"*12.499": 12499, "499,9": 499.9, "1.234.567,89": 1234567.89, "12,499": 12499, "7": 7,
	}
	for in, want := range cases {
		got, ok := ParseAmount(in)
		if !ok || got != want {
			t.Errorf("ParseAmount(%q)=%v,%v want %v", in, got, ok, want)
		}
	}
	if _, ok := ParseAmount("abc"); ok {
		t.Error("abc parsed")
	}
}

func TestCleanNormalizes(t *testing.T) {
	f, missing := Clean(Fields{ProductName: " iPhone 16 ", Category: "Telefon??", Currency: "tl", PurchaseDate: "12.09.2026", Price: -5, WarrantyMonths: 999})
	if f.Category != "phone" || f.Currency != "TRY" || f.PurchaseDate != "2026-09-12" || f.Price != 0 || f.WarrantyMonths != 0 {
		t.Fatalf("clean %+v", f)
	}
	if len(missing) != 2 {
		t.Fatalf("missing %v", missing)
	}
	f, _ = Clean(Fields{PurchaseDate: "31.02.2026", Currency: "€"})
	if f.PurchaseDate != "" || f.Currency != "EUR" || f.Category != "other" {
		t.Fatalf("invalid date should be dropped: %+v", f)
	}
}

func check(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q want %q", name, got, want)
	}
}

func TestRuleExtractorTurkishSerialLabel(t *testing.T) {
	f := ParseText("TEKNOSA\nSONY WH-1000XM5 KULAKLIK 11.999,00\nSERİ NO: 5023417\nTOPLAM *11.999,00 TL")
	if f.SerialNumber != "5023417" {
		t.Fatalf("serial %q", f.SerialNumber)
	}
}
