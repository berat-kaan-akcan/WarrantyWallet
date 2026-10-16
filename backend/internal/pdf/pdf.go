// Package pdf is a tiny dependency-free PDF writer (text, lines, JPEG images) used to build
// the "servis belge paketi" – a single PDF the user can take to an authorized service.
//
// Fonts: the standard Helvetica font with WinAnsiEncoding is used, so the Turkish letters
// ş ğ ı İ Ş Ğ (absent from WinAnsi) are transliterated. Embedding a Unicode TTF is a V2 item.
package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"io"
	"strings"
)

// A4 page size in points.
const (
	PageW = 595.0
	PageH = 842.0
)

type imgObj struct {
	data          []byte
	width, height int
	gray          bool
}

type page struct {
	content bytes.Buffer
	images  []int // indices into doc.images
}

// Doc is an in-memory PDF document.
type Doc struct {
	pages  []*page
	images []imgObj
	Title  string
}

// New creates an empty document.
func New(title string) *Doc { return &Doc{Title: title} }

// AddPage starts a new page; subsequent drawing goes to it.
func (d *Doc) AddPage() { d.pages = append(d.pages, &page{}) }

func (d *Doc) cur() *page {
	if len(d.pages) == 0 {
		d.AddPage()
	}
	return d.pages[len(d.pages)-1]
}

// PageCount returns the number of pages.
func (d *Doc) PageCount() int { return len(d.pages) }

// Text draws a single line of text with its baseline at (x, y) measured from the TOP-left.
func (d *Doc) Text(x, y, size float64, bold bool, s string) {
	font := "F1"
	if bold {
		font = "F2"
	}
	fmt.Fprintf(&d.cur().content, "BT /%s %.1f Tf %.2f %.2f Td (%s) Tj ET\n", font, size, x, PageH-y, escape(encodeWinAnsi(s)))
}

// Wrap draws text wrapped to maxChars per line and returns the y after the last line.
func (d *Doc) Wrap(x, y, size float64, maxChars int, s string) float64 {
	for _, line := range wrap(s, maxChars) {
		d.Text(x, y, size, false, line)
		y += size * 1.4
	}
	return y
}

// Line draws a horizontal rule.
func (d *Doc) Line(x1, y, x2 float64) {
	fmt.Fprintf(&d.cur().content, "0.6 G 0.8 w %.2f %.2f m %.2f %.2f l S 0 G\n", x1, PageH-y, x2, PageH-y)
}

// Rect draws a filled light rectangle (top-left origin).
func (d *Doc) Rect(x, y, w, h float64, gray float64) {
	fmt.Fprintf(&d.cur().content, "%.2f g %.2f %.2f %.2f %.2f re f 0 g\n", gray, x, PageH-y-h, w, h)
}

// Image draws a JPEG or PNG scaled to fit the box (x, y, w, h), preserving aspect ratio.
func (d *Doc) Image(raw []byte, x, y, w, h float64) error {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("unsupported image: %w", err)
	}
	data := raw
	gray := false
	if format != "jpeg" {
		img, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return err
		}
		data = buf.Bytes()
		cfg, _, _ = image.DecodeConfig(bytes.NewReader(data))
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid jpeg: %w", err)
	}
	_, gray = img.(*image.Gray)
	if _, cmyk := img.(*image.CMYK); cmyk {
		// re-encode CMYK as RGB for maximum viewer compatibility
		var buf bytes.Buffer
		rgba := image.NewRGBA(img.Bounds())
		for yy := img.Bounds().Min.Y; yy < img.Bounds().Max.Y; yy++ {
			for xx := img.Bounds().Min.X; xx < img.Bounds().Max.X; xx++ {
				rgba.Set(xx, yy, img.At(xx, yy))
			}
		}
		if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: 85}); err != nil {
			return err
		}
		data = buf.Bytes()
	}
	scale := min(w/float64(cfg.Width), h/float64(cfg.Height))
	dw, dh := float64(cfg.Width)*scale, float64(cfg.Height)*scale
	idx := len(d.images)
	d.images = append(d.images, imgObj{data: data, width: cfg.Width, height: cfg.Height, gray: gray})
	p := d.cur()
	p.images = append(p.images, idx)
	fmt.Fprintf(&p.content, "q %.2f 0 0 %.2f %.2f %.2f cm /Im%d Do Q\n", dw, dh, x, PageH-y-dh, idx)
	return nil
}

// WriteTo serialises the PDF.
func (d *Doc) WriteTo(w io.Writer) (int64, error) {
	if len(d.pages) == 0 {
		d.AddPage()
	}
	var buf bytes.Buffer
	offsets := []int{0}
	obj := func(body string) int {
		offsets = append(offsets, buf.Len())
		n := len(offsets) - 1
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", n, body)
		return n
	}
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	// Reserve: 1 catalog, 2 pages, 3 font regular, 4 font bold, 5 info
	obj("<< /Type /Catalog /Pages 2 0 R >>") // object 1
	pagesIdx := len(offsets)                 // object 2: page tree, written last once kids are known
	offsets = append(offsets, 0)
	f1 := obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	f2 := obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	info := obj(fmt.Sprintf("<< /Title (%s) /Producer (Warranty Wallet) >>", escape(encodeWinAnsi(d.Title))))

	imgNums := make([]int, len(d.images))
	for i, im := range d.images {
		cs := "/DeviceRGB"
		if im.gray {
			cs = "/DeviceGray"
		}
		offsets = append(offsets, buf.Len())
		n := len(offsets) - 1
		fmt.Fprintf(&buf, "%d 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n",
			n, im.width, im.height, cs, len(im.data))
		buf.Write(im.data)
		buf.WriteString("\nendstream\nendobj\n")
		imgNums[i] = n
	}

	var kids []string
	for _, p := range d.pages {
		offsets = append(offsets, buf.Len())
		cn := len(offsets) - 1
		fmt.Fprintf(&buf, "%d 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", cn, p.content.Len(), p.content.String())
		var xo strings.Builder
		for _, ii := range p.images {
			fmt.Fprintf(&xo, "/Im%d %d 0 R ", ii, imgNums[ii])
		}
		pn := obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R /F2 %d 0 R >> /XObject << %s>> >> >>",
			PageW, PageH, cn, f1, f2, xo.String()))
		kids = append(kids, fmt.Sprintf("%d 0 R", pn))
	}
	// pages tree object (number 2)
	offsets[pagesIdx] = buf.Len()
	fmt.Fprintf(&buf, "%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", pagesIdx, strings.Join(kids, " "), len(kids))

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, off := range offsets[1:] {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), info, xref)
	n, err := w.Write(buf.Bytes())
	return int64(n), err
}

func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`, "\r", " ", "\n", " ").Replace(s)
}

var translit = strings.NewReplacer("ş", "s", "Ş", "S", "ğ", "g", "Ğ", "G", "ı", "i", "İ", "I", "₺", "TL")

// encodeWinAnsi converts UTF-8 to WinAnsi bytes (Latin-1 subset); unknown runes → '?'.
func encodeWinAnsi(s string) string {
	s = translit.Replace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x80:
			b.WriteRune(r)
		case r >= 0xA0 && r <= 0xFF:
			b.WriteByte(byte(r))
		case r == '€':
			b.WriteByte(0x80)
		case r == '–' || r == '—':
			b.WriteByte('-')
		case r == '•':
			b.WriteByte(0x95)
		case r == '“' || r == '”':
			b.WriteByte('"')
		case r == '’' || r == '‘':
			b.WriteByte('\'')
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

func wrap(s string, max int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		line := ""
		for _, w := range words {
			if len([]rune(line))+len([]rune(w))+1 > max && line != "" {
				out = append(out, line)
				line = w
				continue
			}
			if line == "" {
				line = w
			} else {
				line += " " + w
			}
		}
		out = append(out, line)
	}
	return out
}
