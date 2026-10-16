package extract

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RuleExtractor is a deterministic, offline parser tuned for Turkish receipts/invoices.
// It only works on text (OCR output); it cannot read images.
type RuleExtractor struct{}

// Name implements ReceiptExtractor.
func (RuleExtractor) Name() string { return "rules" }

// Extract implements ReceiptExtractor.
func (RuleExtractor) Extract(_ context.Context, in Input) (Result, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return Result{}, ErrEmptyInput
	}
	f := ParseText(text)
	f, missing := Clean(f)
	return Result{Fields: f, Provider: "rules", RawText: text, Missing: missing}, nil
}

// Known sellers (display form) and brands, matched on folded text.
var knownSellers = []string{"MediaMarkt", "Teknosa", "Vatan Bilgisayar", "Hepsiburada", "Trendyol", "Amazon",
	"n11", "Apple", "Samsung", "Arçelik", "Beko", "Vestel", "Boyner", "Migros", "CarrefourSA", "A101", "BİM",
	"Gratis", "İtopya", "İnceHesap", "PttAVM", "Dyson", "Philips", "Turkcell", "Vodafone", "Türk Telekom",
	"Gürgençler", "Bimeks", "Koçtaş", "IKEA", "Decathlon", "Xiaomi Store"}

var knownBrands = []string{"Apple", "Samsung", "Xiaomi", "Huawei", "Oppo", "Realme", "OnePlus", "Google",
	"ASUS", "Acer", "Lenovo", "HP", "Dell", "MSI", "Monster", "Casper", "Huawei", "LG", "Sony", "Philips",
	"Bosch", "Siemens", "Arçelik", "Beko", "Vestel", "Grundig", "Profilo", "Tefal", "Delonghi", "De'Longhi",
	"Nespresso", "Krups", "Fakir", "Arzum", "Dyson", "Karcher", "JBL", "Bose", "Sennheiser", "Logitech",
	"Razer", "SteelSeries", "HyperX", "Canon", "Nikon", "Fujifilm", "GoPro", "Nintendo", "Microsoft",
	"Garmin", "Amazfit", "Anker", "TP-Link", "BenQ", "AOC", "ViewSonic", "Gigabyte", "Toshiba", "TCL",
	"Braun", "Rowenta", "Sinbo", "Kumtel", "Smeg", "Electrolux", "Whirlpool", "Miele", "Marshall", "Lenovo"}

var categoryKeywords = []struct {
	cat   string
	words []string
}{
	{"monitor", []string{"monitor", "monitör", "ekran 27", "vg27", "odyssey"}},
	{"laptop", []string{"laptop", "notebook", "dizustu", "macbook", "thinkpad", "zenbook", "vivobook", "ideapad", "tuf gaming f", "rog strix g"}},
	{"headphones", []string{"kulaklik", "headphone", "earbuds", "airpods", "buds", "wh-1000", "headset"}},
	{"phone", []string{"telefon", "iphone", "galaxy s", "galaxy a", "redmi", "pixel", "akilli telefon", "smartphone"}},
	{"tablet", []string{"tablet", "ipad", "galaxy tab", "matepad"}},
	{"tv", []string{"televizyon", "smart tv", "oled tv", "qled", " tv "}},
	{"console", []string{"playstation", "ps5", "ps4", "xbox", "nintendo switch"}},
	{"wearable", []string{"akilli saat", "smart watch", "smartwatch", "apple watch", "galaxy watch", "watch gt", "mi band"}},
	{"camera", []string{"kamera", "camera", "fotograf makinesi", "gopro", "eos "}},
	{"appliance", []string{"buzdolabi", "camasir makinesi", "bulasik makinesi", "kurutma makinesi", "firin", "klima"}},
	{"small_appliance", []string{"kahve makinesi", "espresso", "blender", "supurge", "tost makinesi", "airfryer", "air fryer", "fritoz", "su isitici", "kettle", "mikser", "utu", "sac kurutma"}},
}

var (
	reDateNum   = regexp.MustCompile(`\b(\d{1,2})[./-](\d{1,2})[./-](\d{4}|\d{2})\b`)
	reDateISO   = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	reDateWords = regexp.MustCompile(`(?i)\b(\d{1,2})\s+(ocak|subat|şubat|mart|nisan|mayis|mayıs|haziran|temmuz|agustos|ağustos|eylul|eylül|ekim|kasim|kasım|aralik|aralık)\s+(\d{4})\b`)
	reAmount    = regexp.MustCompile(`(\d{1,3}(?:[.\s]\d{3})+(?:,\d{1,2})?|\d+(?:[.,]\d{1,2})?)`)
	reWarranty  = regexp.MustCompile(`(\d{1,3})\s*(ay|month|months|yil|yıl|year|years|sene)\b`)
	reSerial    = regexp.MustCompile(`(?i)(?:s/n|sn:|seri\s*no|seri\s*numarasi|seri\s*numarası|serial(?:\s*no| number)?|imei)\s*[:#.]?\s*([A-Z0-9][A-Z0-9-]{4,})`)
	reModelTok  = regexp.MustCompile(`\b([A-Z]{1,6}[- ]?\d{2,5}[A-Z0-9-]*|\d{2,4}[A-Z]{1,4}\d*)\b`)
)

var trMonths = map[string]time.Month{"ocak": 1, "subat": 2, "mart": 3, "nisan": 4, "mayis": 5, "haziran": 6,
	"temmuz": 7, "agustos": 8, "eylul": 9, "ekim": 10, "kasim": 11, "aralik": 12}

// fold lowercases and strips Turkish diacritics for matching ("GARANTİ" → "garanti").
func fold(s string) string {
	r := strings.NewReplacer("İ", "i", "I", "i", "ı", "i", "Ş", "s", "ş", "s", "Ğ", "g", "ğ", "g",
		"Ü", "u", "ü", "u", "Ö", "o", "ö", "o", "Ç", "c", "ç", "c")
	return strings.ToLower(r.Replace(s))
}

// trASCII maps Turkish letters to their ASCII look-alikes, keeping case ("SERİ" → "SERI"),
// so case-insensitive ASCII regexes match Turkish labels.
var trASCII = strings.NewReplacer("İ", "I", "ı", "i", "Ş", "S", "ş", "s", "Ğ", "G", "ğ", "g",
	"Ü", "U", "ü", "u", "Ö", "O", "ö", "o", "Ç", "C", "ç", "c")

// ParseText applies all heuristics to OCR text.
func ParseText(text string) Fields {
	lines := splitLines(text)
	folded := fold(text)
	f := Fields{}
	f.PurchaseDate = findDate(text)
	f.Price, f.Currency = findTotal(lines)
	f.WarrantyMonths = findWarranty(lines)
	f.Seller = findSeller(lines, folded)
	f.Brand = findBrand(lines, f.Seller)
	f.ProductName, f.Model = findProduct(lines, f.Brand)
	if m := reSerial.FindStringSubmatch(trASCII.Replace(text)); m != nil {
		f.SerialNumber = strings.ToUpper(m[1])
	}
	f.Category = GuessCategory(f.ProductName + " " + f.Model + " " + text)
	return f
}

func splitLines(text string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// GuessCategory maps keywords to a category slug ("" if none).
func GuessCategory(s string) string {
	fs := " " + fold(s) + " "
	for _, ck := range categoryKeywords {
		for _, w := range ck.words {
			if strings.Contains(fs, w) {
				return ck.cat
			}
		}
	}
	return ""
}

func findDate(text string) string {
	type cand struct {
		pos int
		val string
	}
	var cands []cand
	for _, m := range reDateISO.FindAllStringSubmatchIndex(text, -1) {
		y, _ := strconv.Atoi(text[m[2]:m[3]])
		mo, _ := strconv.Atoi(text[m[4]:m[5]])
		d, _ := strconv.Atoi(text[m[6]:m[7]])
		if v, ok := mkDate(y, mo, d); ok {
			cands = append(cands, cand{m[0], v})
		}
	}
	for _, m := range reDateNum.FindAllStringSubmatchIndex(text, -1) {
		d, _ := strconv.Atoi(text[m[2]:m[3]])
		mo, _ := strconv.Atoi(text[m[4]:m[5]])
		y, _ := strconv.Atoi(text[m[6]:m[7]])
		if y < 100 {
			y += 2000
		}
		if v, ok := mkDate(y, mo, d); ok {
			cands = append(cands, cand{m[0], v})
		}
	}
	for _, m := range reDateWords.FindAllStringSubmatchIndex(text, -1) {
		d, _ := strconv.Atoi(text[m[2]:m[3]])
		mo := trMonths[fold(text[m[4]:m[5]])]
		y, _ := strconv.Atoi(text[m[6]:m[7]])
		if v, ok := mkDate(y, int(mo), d); ok {
			cands = append(cands, cand{m[0], v})
		}
	}
	if len(cands) == 0 {
		return ""
	}
	// Prefer a date on a line mentioning "tarih"/"date"; else the first date in the document.
	low := fold(text)
	for _, c := range cands {
		start := strings.LastIndex(low[:c.pos], "\n") + 1
		if strings.Contains(low[start:c.pos], "tarih") || strings.Contains(low[start:c.pos], "date") {
			return c.val
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].pos < cands[j].pos })
	return cands[0].val
}

func mkDate(y, m, d int) (string, bool) {
	if y < 1990 || y > 2100 || m < 1 || m > 12 || d < 1 || d > 31 {
		return "", false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d {
		return "", false
	}
	return t.Format("2006-01-02"), true
}

// parseAnyDate accepts YYYY-MM-DD, DD.MM.YYYY, DD/MM/YYYY.
func parseAnyDate(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if m := reDateISO.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return mkDate(y, mo, d)
	}
	if m := reDateNum.FindStringSubmatch(s); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		if y < 100 {
			y += 2000
		}
		return mkDate(y, mo, d)
	}
	return "", false
}

// ParseAmount parses "12.499,00", "12,499.00", "12499.90", "1 299,90", "*12.499".
func ParseAmount(s string) (float64, bool) {
	s = strings.TrimSpace(strings.Trim(s, "*₺$€ "))
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0, false
	}
	lastDot, lastComma := strings.LastIndex(s, "."), strings.LastIndex(s, ",")
	switch {
	case lastDot >= 0 && lastComma >= 0:
		if lastComma > lastDot { // 12.499,00 (TR)
			s = strings.ReplaceAll(s, ".", "")
			s = strings.Replace(s, ",", ".", 1)
		} else { // 12,499.00 (EN)
			s = strings.ReplaceAll(s, ",", "")
		}
	case lastComma >= 0:
		if strings.Count(s, ",") > 1 || len(s)-lastComma-1 == 3 {
			s = strings.ReplaceAll(s, ",", "") // 12,499 / 1,299,000 thousands
		} else {
			s = strings.Replace(s, ",", ".", 1) // 499,90 decimal
		}
	case lastDot >= 0:
		if len(s)-lastDot-1 == 3 { // 12.499 → thousands separator in TR
			s = strings.ReplaceAll(s, ".", "")
		} else if strings.Count(s, ".") > 1 {
			parts := strings.Split(s, ".")
			s = strings.Join(parts[:len(parts)-1], "") + "." + parts[len(parts)-1]
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || v > 1e9 {
		return 0, false
	}
	return math.Round(v*100) / 100, true
}

func findTotal(lines []string) (float64, string) {
	currency := ""
	best := 0.0
	bestScore := -1
	for _, l := range lines {
		fl := fold(l)
		score := 0
		switch {
		case strings.Contains(fl, "genel toplam"), strings.Contains(fl, "odenecek"), strings.Contains(fl, "grand total"):
			score = 3
		case strings.Contains(fl, "toplam"), strings.Contains(fl, "total"), strings.Contains(fl, "tutar"):
			score = 2
		}
		if strings.Contains(fl, "kdv") && !strings.Contains(fl, "dahil") && score < 3 {
			score = 0 // "TOPLAM KDV" is tax, not the total
		}
		if strings.Contains(fl, "ara toplam") {
			score = 1
		}
		if c := detectCurrency(l); c != "" && currency == "" {
			currency = c
		}
		if score == 0 {
			continue
		}
		for _, m := range reAmount.FindAllString(stripDates(l), -1) {
			if v, ok := ParseAmount(m); ok && v > 0 {
				if score > bestScore || (score == bestScore && v > best) {
					best, bestScore = v, score
				}
			}
		}
	}
	if best == 0 { // no labelled total → largest money-looking amount
		for _, l := range lines {
			if detectCurrency(l) == "" && !strings.Contains(l, ",") {
				continue
			}
			for _, m := range reAmount.FindAllString(stripDates(l), -1) {
				if v, ok := ParseAmount(m); ok && v > best && (strings.ContainsAny(m, ",.") || detectCurrency(l) != "") {
					best = v
				}
			}
		}
	}
	if currency == "" {
		currency = "TRY"
	}
	return best, currency
}

func stripDates(l string) string {
	l = reDateISO.ReplaceAllString(l, " ")
	l = reDateNum.ReplaceAllString(l, " ")
	return regexp.MustCompile(`\b\d{1,2}:\d{2}(:\d{2})?\b`).ReplaceAllString(l, " ")
}

func detectCurrency(l string) string {
	fl := " " + fold(l) + " "
	switch {
	case strings.Contains(l, "₺"), strings.Contains(fl, " tl "), strings.Contains(fl, " try "), strings.HasSuffix(strings.TrimSpace(fl), "tl"):
		return "TRY"
	case strings.Contains(l, "€"), strings.Contains(fl, " eur "):
		return "EUR"
	case strings.Contains(l, "$"), strings.Contains(fl, " usd "):
		return "USD"
	}
	return ""
}

func findWarranty(lines []string) int {
	for i, l := range lines {
		fl := fold(l)
		if !strings.Contains(fl, "garanti") && !strings.Contains(fl, "warranty") {
			continue
		}
		ctx := fl
		if i+1 < len(lines) {
			ctx += " " + fold(lines[i+1])
		}
		if m := reWarranty.FindStringSubmatch(ctx); m != nil {
			n, _ := strconv.Atoi(m[1])
			switch m[2] {
			case "yil", "year", "years", "sene":
				n *= 12
			}
			if n > 0 && n <= 240 {
				return n
			}
		}
	}
	return 0
}

func findSeller(lines []string, folded string) string {
	for _, s := range knownSellers {
		if strings.Contains(folded, fold(s)) {
			return s
		}
	}
	// Receipt header: first line that looks like a business name.
	for _, l := range lines[:min(len(lines), 3)] {
		fl := fold(l)
		if reDateNum.MatchString(l) || strings.Contains(fl, "fis") || strings.Contains(fl, "fatura") || len(l) < 3 {
			continue
		}
		if strings.ContainsAny(l, "0123456789") && len(reAmount.FindAllString(l, -1)) > 1 {
			continue
		}
		return strings.TrimSpace(l)
	}
	return ""
}

func findBrand(lines []string, seller string) string {
	fs := fold(seller)
	for _, l := range lines {
		fl := " " + fold(l) + " "
		if fs != "" && strings.Contains(fl, fs) {
			continue
		}
		for _, b := range knownBrands {
			fb := fold(b)
			if strings.Contains(fl, " "+fb+" ") {
				return b
			}
		}
	}
	return ""
}

func findProduct(lines []string, brand string) (name, model string) {
	if brand == "" {
		return "", ""
	}
	fb := fold(brand)
	for _, l := range lines {
		fl := " " + fold(l) + " "
		if !strings.Contains(fl, " "+fb+" ") {
			continue
		}
		// skip lines that are clearly not item lines
		if strings.Contains(fl, "garanti") || strings.Contains(fl, "yetkili") || strings.Contains(fl, "servis") {
			continue
		}
		clean := l
		// drop trailing quantity/price columns: "ASUS TUF VG27AQ   1 AD  12.499,00"
		clean = regexp.MustCompile(`(?i)\s+(\d+\s*(ad|adet|x)\b.*|[*x]\s*\d.*|\d{1,3}(?:[.\s]\d{3})*,\d{2}.*)$`).ReplaceAllString(clean, "")
		clean = strings.TrimSpace(strings.Trim(clean, "-*:"))
		idx := strings.Index(fold(clean), fb)
		if idx > 0 && idx < len(clean) {
			clean = clean[idx:]
		}
		if m := reModelTok.FindString(clean[min(len(brand), len(clean)):]); m != "" {
			model = strings.TrimSpace(m)
		}
		return strings.TrimSpace(clean), model
	}
	return "", ""
}
