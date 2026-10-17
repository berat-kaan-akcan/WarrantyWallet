package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

func testImage(t *testing.T, format string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for x := 0; x < 40; x++ {
		img.Set(x, 10, color.RGBA{255, 0, 0, 255})
	}
	var buf bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// validateXref checks every xref offset points at "N 0 obj".
func validateXref(t *testing.T, b []byte) int {
	t.Helper()
	s := string(b)
	if !strings.HasPrefix(s, "%PDF-1.4") || !strings.HasSuffix(s, "%%EOF\n") {
		t.Fatal("bad header/trailer")
	}
	m := regexp.MustCompile(`startxref\n(\d+)\n`).FindStringSubmatch(s)
	xref, _ := strconv.Atoi(m[1])
	if !strings.HasPrefix(s[xref:], "xref\n") {
		t.Fatal("startxref does not point to xref")
	}
	lines := strings.Split(s[xref:], "\n")
	var count int
	_, _ = strconv.Atoi(strings.Fields(lines[1])[1])
	count, _ = strconv.Atoi(strings.Fields(lines[1])[1])
	for i := 1; i < count; i++ {
		off, _ := strconv.Atoi(strings.Fields(lines[2+i])[0])
		want := strconv.Itoa(i) + " 0 obj"
		if !strings.HasPrefix(s[off:], want) {
			t.Fatalf("xref entry %d -> %q", i, s[off:off+12])
		}
	}
	return count
}

func TestDocWithImages(t *testing.T) {
	d := New("Test")
	d.AddPage()
	d.Text(40, 40, 12, true, "Şirket İğdır ışık – € (test)")
	d.Line(40, 50, 500)
	if err := d.Image(testImage(t, "jpeg"), 40, 60, 200, 200); err != nil {
		t.Fatal(err)
	}
	d.AddPage()
	if err := d.Image(testImage(t, "png"), 40, 60, 200, 200); err != nil {
		t.Fatal(err)
	}
	if err := d.Image([]byte("not an image"), 0, 0, 1, 1); err == nil {
		t.Fatal("expected error")
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	validateXref(t, buf.Bytes())
	s := buf.String()
	if !strings.Contains(s, "/Count 2") || strings.Count(s, "/Subtype /Image") != 2 {
		t.Fatal("pages/images missing")
	}
	if !strings.Contains(s, "(Sirket Igdir isik - \x80 \\(test\\)) Tj") {
		t.Fatalf("text encoding wrong")
	}
}

func TestServicePack(t *testing.T) {
	p := domain.Product{Name: "ASUS TUF VG27AQ", Brand: "ASUS", Model: "VG27AQ", Seller: "MediaMarkt", SerialNumber: "M1LMQS",
		PurchaseDate: domain.NewDate(2026, 9, 12), Price: 12499, Currency: "TRY", WarrantyMonths: 24}
	p.Normalize()
	resolved := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	sr := &domain.ServiceRecord{Problem: domain.ProblemScreenLines, Description: "Ekranın sol tarafında dikey çizgiler var.",
		Status: domain.ServiceResolved, ServiceName: "ASUS Yetkili Servis", Cost: 750, ResolvedAt: &resolved}
	var events []domain.ServiceEvent
	for i := 0; i < 40; i++ { // long timeline must flow onto a new page
		events = append(events, domain.ServiceEvent{Status: domain.ServiceInService, Note: fmt.Sprintf("adim %d", i),
			CreatedAt: time.Date(2026, 10, 17, 10, i, 0, 0, time.UTC)})
	}
	atts := []Attachment{
		{Doc: domain.Document{Type: domain.DocInvoice, ContentType: "image/jpeg", OriginalName: "fatura.jpg"}, Data: testImage(t, "jpeg")},
		{Doc: domain.Document{Type: domain.DocWarranty, ContentType: "application/pdf", OriginalName: "garanti.pdf"}, Data: []byte("%PDF")},
	}
	b, err := ServicePack(p, sr, events, atts, domain.NewDate(2026, 10, 16), "Berat")
	if err != nil {
		t.Fatal(err)
	}
	validateXref(t, b)
	if out := os.Getenv("PDF_OUT"); out != "" { // manual check: qpdf --check / pdftotext
		_ = os.WriteFile(out, b, 0o644)
	}
	s := string(b)
	for _, want := range []string{"/Count 3", "ASUS TUF VG27AQ", "750,00 TRY", "Servis s\xfcreci", "17.10.2026 13:00 Serviste - adim 0", "adim 39", "20.10.2026", "12.499,00 TRY", "12.09.2028", "Ekranda \xe7izgiler", "garanti.pdf", "[ X ]  Fatura"} {
		if !strings.Contains(s, want) {
			t.Errorf("pack missing %q", want)
		}
	}
}

func TestChecklistAndMoney(t *testing.T) {
	c := Checklist(domain.Product{}, nil)
	for _, it := range c {
		if it.OK {
			t.Fatalf("empty product should have nothing: %+v", it)
		}
	}
	c = Checklist(domain.Product{SerialNumber: "x", Model: "y"}, []domain.Document{{Type: domain.DocInvoice}, {Type: domain.DocWarranty}})
	for _, it := range c {
		if !it.OK {
			t.Fatalf("expected all ok: %+v", it)
		}
	}
	for in, want := range map[float64]string{12499: "12.499,00", 999.5: "999,50", 1234567.891: "1.234.567,89", 0: "0,00"} {
		if got := formatMoney(in); got != want {
			t.Errorf("formatMoney(%v)=%s want %s", in, got, want)
		}
	}
}
