package pdf

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

// Attachment is a stored document to include in the pack.
type Attachment struct {
	Doc  domain.Document
	Data []byte // nil if it could not be loaded
}

// ChecklistItem is one document the service will ask for.
type ChecklistItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	OK    bool   `json:"ok"`
}

// Checklist computes the "servise götürülecek belgeler" list.
func Checklist(p domain.Product, docs []domain.Document) []ChecklistItem {
	has := func(t domain.DocumentType) bool {
		for _, d := range docs {
			if d.Type == t {
				return true
			}
		}
		return false
	}
	return []ChecklistItem{
		{"invoice", "Fatura / fiş", has(domain.DocInvoice)},
		{"warranty", "Garanti belgesi", has(domain.DocWarranty)},
		{"serial_number", "Seri numarası", p.SerialNumber != ""},
		{"model", "Model bilgisi", p.Model != "" || p.Brand != ""},
	}
}

var problemTR = map[domain.ProblemType]string{
	domain.ProblemWontTurnOn:     "Açılmıyor",
	domain.ProblemScreenLines:    "Ekranda çizgiler",
	domain.ProblemPhysicalDamage: "Fiziksel hasar",
	domain.ProblemOther:          "Diğer",
}

// ProblemLabel returns the Turkish label for a problem type.
func ProblemLabel(p domain.ProblemType) string {
	if s, ok := problemTR[p]; ok {
		return s
	}
	return string(p)
}

// ServicePack builds the PDF: a cover page with product + warranty info + checklist
// (+ optional latest service record), followed by one page per image document.
func ServicePack(p domain.Product, sr *domain.ServiceRecord, atts []Attachment, today domain.Date, owner string) ([]byte, error) {
	d := New("Servis Belge Paketi - " + p.Name)
	d.AddPage()
	d.Rect(0, 0, PageW, 70, 0.93)
	d.Text(40, 42, 20, true, "Warranty Wallet - Servis Belge Paketi")
	d.Text(40, 60, 9, false, "Olusturulma: "+time.Now().Format("02.01.2006 15:04"))

	y := 110.0
	row := func(label, value string) {
		d.Text(40, y, 11, true, label)
		d.Text(200, y, 11, false, value)
		y += 20
	}
	d.Text(40, y, 14, true, "Ürün bilgileri")
	y += 24
	row("Ürün", p.Name)
	row("Marka / Model", strings.Trim(p.Brand+" / "+p.Model, " /"))
	row("Seri numarası", dash(p.SerialNumber))
	row("Satıcı", dash(p.Seller))
	row("Satın alma tarihi", fmtDate(p.PurchaseDate))
	row("Fiyat", fmt.Sprintf("%s %s", formatMoney(p.Price), p.Currency))
	row("Garanti süresi", fmt.Sprintf("%d ay", p.WarrantyMonths))
	row("Garanti bitişi", fmtDate(p.WarrantyEnd))
	status := "Garanti kapsamında"
	if !domain.IsCovered(p.WarrantyEnd, today) {
		status = "Garanti süresi dolmuş"
	} else {
		status += fmt.Sprintf(" (%d gün kaldı)", domain.DaysLeft(p.WarrantyEnd, today))
	}
	row("Durum", status)
	if owner != "" {
		row("Ürün sahibi", owner)
	}

	if sr != nil {
		y += 10
		d.Line(40, y, PageW-40)
		y += 24
		d.Text(40, y, 14, true, "Arıza bilgisi")
		y += 24
		row("Sorun", ProblemLabel(sr.Problem))
		if sr.Description != "" {
			d.Text(40, y, 11, true, "Açıklama")
			y = d.Wrap(200, y, 11, 60, sr.Description) + 6
		}
		if sr.ServiceName != "" {
			row("Servis", sr.ServiceName)
		}
	}

	y += 10
	d.Line(40, y, PageW-40)
	y += 24
	docs := make([]domain.Document, 0, len(atts))
	for _, a := range atts {
		docs = append(docs, a.Doc)
	}
	d.Text(40, y, 14, true, "Belge kontrol listesi")
	y += 24
	for _, c := range Checklist(p, docs) {
		mark := "[ X ]"
		if !c.OK {
			mark = "[   ]"
		}
		d.Text(40, y, 11, false, mark+"  "+c.Label)
		y += 18
	}

	var images []Attachment
	var skipped []string
	for _, a := range atts {
		if a.Data == nil || !strings.HasPrefix(a.Doc.ContentType, "image/") {
			skipped = append(skipped, a.Doc.OriginalName)
			continue
		}
		images = append(images, a)
	}
	if len(skipped) > 0 {
		y += 10
		d.Wrap(40, y, 9, 100, "Pakete gömülemeyen dosyalar (ayrıca yazdırın): "+strings.Join(skipped, ", "))
	}
	for _, a := range images {
		d.AddPage()
		d.Text(40, 40, 13, true, docTitle(a.Doc.Type)+" - "+a.Doc.OriginalName)
		if err := d.Image(a.Data, 40, 60, PageW-80, PageH-100); err != nil {
			d.Text(40, 80, 11, false, "Görsel eklenemedi: "+err.Error())
		}
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func docTitle(t domain.DocumentType) string {
	switch t {
	case domain.DocInvoice:
		return "Fatura"
	case domain.DocWarranty:
		return "Garanti Belgesi"
	case domain.DocService:
		return "Servis Belgesi"
	default:
		return "Fotoğraf"
	}
}

func fmtDate(d domain.Date) string {
	if d.IsZero() {
		return "-"
	}
	return d.Time().Format("02.01.2006")
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// formatMoney renders 12499.5 as "12.499,50" (Turkish style).
func formatMoney(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	intPart, frac := s[:len(s)-3], s[len(s)-2:]
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 && c != '-' {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return b.String() + "," + frac
}
