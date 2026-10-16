package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/auth"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/extract"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/storage"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store/memory"
)

// fixed "now": 16 Oct 2026, 12:00 Türkiye time
var testNow = time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

type harness struct {
	t   *testing.T
	srv *httptest.Server
	api *Server
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	files, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := &Server{
		Store: memory.New(), Files: files,
		Extractor:   &extract.Fallback{Primary: &extract.GeminiExtractor{}, Secondary: extract.RuleExtractor{}},
		Tokens:      auth.Tokens{Secret: []byte("test-secret-0123456789"), TTL: time.Hour, Now: func() time.Time { return testNow }},
		Hasher:      auth.Hasher{Iterations: 1000},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		MaxUpload:   1 << 20,
		AdminEmails: []string{"admin@ww.test"},
		CORSOrigins: "*",
		Now:         func() time.Time { return testNow },
	}
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return &harness{t: t, srv: srv, api: api}
}

type resp struct {
	Status int
	Body   []byte
	Header http.Header
}

func (r resp) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatalf("not json (%d): %s", r.Status, r.Body)
	}
	return m
}

func (h *harness) do(method, path, token string, body any) resp {
	h.t.Helper()
	var rd io.Reader
	ct := ""
	switch b := body.(type) {
	case nil:
	case string:
		rd, ct = strings.NewReader(b), "application/json"
	case *multipartBody:
		rd, ct = &b.buf, b.ct
	default:
		js, _ := json.Marshal(b)
		rd, ct = bytes.NewReader(js), "application/json"
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, rd)
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{res.StatusCode, b, res.Header}
}

func (h *harness) register(email, name string) string {
	h.t.Helper()
	r := h.do("POST", "/api/v1/auth/register", "", map[string]string{"email": email, "password": "supersecret", "name": name})
	if r.Status != 201 {
		h.t.Fatalf("register %d %s", r.Status, r.Body)
	}
	return r.json(h.t)["token"].(string)
}

type multipartBody struct {
	buf bytes.Buffer
	ct  string
}

func newMultipart(fields map[string]string, fileField, fileName string, data []byte) *multipartBody {
	mb := &multipartBody{}
	w := multipart.NewWriter(&mb.buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if fileField != "" {
		fw, _ := w.CreateFormFile(fileField, fileName)
		_, _ = fw.Write(data)
	}
	_ = w.Close()
	mb.ct = w.FormDataContentType()
	return mb
}

func jpegBytes() []byte {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil)
	return b.Bytes()
}

var monitor = map[string]any{
	"name": "ASUS TUF VG27AQ", "brand": "ASUS", "model": "VG27AQ", "category": "monitor", "seller": "MediaMarkt",
	"purchase_date": "2026-09-12", "price": 12499, "currency": "TRY", "warranty_months": 24, "serial_number": "M1LMQS123",
}

func TestHealth(t *testing.T) {
	h := newHarness(t)
	r := h.do("GET", "/healthz", "", nil)
	if r.Status != 200 || r.json(t)["status"] != "ok" || r.json(t)["extractor"] != "rules" {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
	if r.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing")
	}
	if r := h.do("GET", "/nope", "", nil); r.Status != 404 {
		t.Fatal("unknown route should 404")
	}
}

func TestAuthFlow(t *testing.T) {
	h := newHarness(t)
	tok := h.register("Berat@Example.com", "Berat Kaan")

	me := h.do("GET", "/api/v1/me", tok, nil)
	if me.Status != 200 || me.json(t)["email"] != "berat@example.com" || me.json(t)["role"] != "user" {
		t.Fatalf("me %d %s", me.Status, me.Body)
	}
	if strings.Contains(string(me.Body), "password") {
		t.Fatal("password hash leaked")
	}
	// duplicate email (case-insensitive)
	if r := h.do("POST", "/api/v1/auth/register", "", map[string]string{"email": "berat@example.com", "password": "supersecret"}); r.Status != 409 {
		t.Fatalf("dup %d", r.Status)
	}
	// validation
	r := h.do("POST", "/api/v1/auth/register", "", map[string]string{"email": "not-an-email", "password": "short"})
	if r.Status != 422 {
		t.Fatalf("validation %d", r.Status)
	}
	fields := r.json(t)["error"].(map[string]any)["fields"].(map[string]any)
	if fields["email"] == nil || fields["password"] == nil {
		t.Fatalf("fields %v", fields)
	}
	// unknown JSON field rejected
	if r := h.do("POST", "/api/v1/auth/login", "", `{"email":"a@b.c","pasword":"x"}`); r.Status != 400 {
		t.Fatalf("unknown field %d", r.Status)
	}
	// login ok / wrong password / unknown email give same error
	if r := h.do("POST", "/api/v1/auth/login", "", map[string]string{"email": "BERAT@example.com", "password": "supersecret"}); r.Status != 200 || r.json(t)["token"] == "" {
		t.Fatalf("login %d %s", r.Status, r.Body)
	}
	bad1 := h.do("POST", "/api/v1/auth/login", "", map[string]string{"email": "berat@example.com", "password": "wrongpass"})
	bad2 := h.do("POST", "/api/v1/auth/login", "", map[string]string{"email": "ghost@example.com", "password": "wrongpass"})
	if bad1.Status != 401 || bad2.Status != 401 || string(bad1.Body) != string(bad2.Body) {
		t.Fatalf("login errors differ: %s vs %s", bad1.Body, bad2.Body)
	}
	// protected routes
	if r := h.do("GET", "/api/v1/products", "", nil); r.Status != 401 {
		t.Fatal("no token should be 401")
	}
	if r := h.do("GET", "/api/v1/products", "garbage", nil); r.Status != 401 {
		t.Fatal("bad token should be 401")
	}
	// admin email gets admin role
	atok := h.register("admin@ww.test", "Admin")
	if r := h.do("GET", "/api/v1/me", atok, nil); r.json(t)["role"] != "admin" {
		t.Fatal("admin role not assigned")
	}
}

func TestAuthRateLimit(t *testing.T) {
	h := newHarness(t)
	var last int
	for i := 0; i < 12; i++ {
		last = h.do("POST", "/api/v1/auth/login", "", map[string]string{"email": "x@y.z", "password": "whatever1"}).Status
	}
	if last != 429 {
		t.Fatalf("expected 429 after burst, got %d", last)
	}
}

func TestProductCRUDAndIsolation(t *testing.T) {
	h := newHarness(t)
	a := h.register("a@ww.test", "Ayşe")
	b := h.register("b@ww.test", "Bora")

	r := h.do("POST", "/api/v1/products", a, monitor)
	if r.Status != 201 {
		t.Fatalf("create %d %s", r.Status, r.Body)
	}
	p := r.json(t)
	id := p["id"].(string)
	if p["warranty_end"] != "2028-09-12" || p["status"] != "active" || p["days_left"].(float64) != 697 {
		t.Fatalf("computed fields wrong: %v", p)
	}
	// client cannot override warranty_end
	if r := h.do("POST", "/api/v1/products", a, map[string]any{"name": "X", "purchase_date": "2026-01-01", "warranty_end": "2099-01-01"}); r.Status != 400 {
		t.Fatalf("warranty_end must not be accepted: %d", r.Status)
	}
	// default warranty is 24 months when omitted
	r = h.do("POST", "/api/v1/products", a, map[string]any{"name": "Sony WH-1000XM5", "brand": "Sony", "category": "headphones", "purchase_date": "2024-11-01", "price": 8999.9})
	if r.Status != 201 || r.json(t)["warranty_months"].(float64) != 24 || r.json(t)["status"] != "expiring_soon" {
		t.Fatalf("default warranty: %d %s", r.Status, r.Body)
	}
	sonyID := r.json(t)["id"].(string)
	// expired product
	h.do("POST", "/api/v1/products", a, map[string]any{"name": "Eski Telefon", "category": "phone", "purchase_date": "2020-01-01", "warranty_months": 24, "price": 5000})

	// validation
	r = h.do("POST", "/api/v1/products", a, map[string]any{"name": "", "purchase_date": "2030-01-01", "price": -1, "category": "spaceship", "warranty_months": 500})
	if r.Status != 422 {
		t.Fatalf("validation %d", r.Status)
	}
	f := r.json(t)["error"].(map[string]any)["fields"].(map[string]any)
	for _, k := range []string{"name", "purchase_date", "price", "category", "warranty_months"} {
		if f[k] == nil {
			t.Errorf("missing field error %s: %v", k, f)
		}
	}

	// list + filters
	if r := h.do("GET", "/api/v1/products", a, nil); r.json(t)["count"].(float64) != 3 {
		t.Fatalf("list %s", r.Body)
	}
	if r := h.do("GET", "/api/v1/products?category=monitor", a, nil); r.json(t)["count"].(float64) != 1 {
		t.Fatal("category filter")
	}
	if r := h.do("GET", "/api/v1/products?status=expired", a, nil); r.json(t)["count"].(float64) != 1 {
		t.Fatal("status filter")
	}
	if r := h.do("GET", "/api/v1/products?q=sony", a, nil); r.json(t)["count"].(float64) != 1 {
		t.Fatal("search")
	}
	// expiring soon list sorted, with days left
	r = h.do("GET", "/api/v1/products/expiring?days=90", a, nil)
	items := r.json(t)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != sonyID || items[0].(map[string]any)["days_left"].(float64) != 16 {
		t.Fatalf("expiring %s", r.Body)
	}

	// isolation: b cannot see/update/delete a's product
	if r := h.do("GET", "/api/v1/products/"+id, b, nil); r.Status != 404 {
		t.Fatalf("b read a's product: %d", r.Status)
	}
	if r := h.do("PUT", "/api/v1/products/"+id, b, monitor); r.Status != 404 {
		t.Fatal("b updated a's product")
	}
	if r := h.do("DELETE", "/api/v1/products/"+id, b, nil); r.Status != 404 {
		t.Fatal("b deleted a's product")
	}
	if r := h.do("GET", "/api/v1/products", b, nil); r.json(t)["count"].(float64) != 0 {
		t.Fatal("b sees products")
	}

	// update recomputes the end date and keeps months when omitted
	upd := map[string]any{"name": "ASUS TUF Gaming VG27AQ", "brand": "ASUS", "category": "monitor", "purchase_date": "2026-09-12", "price": 11999, "warranty_months": 36}
	r = h.do("PUT", "/api/v1/products/"+id, a, upd)
	if r.Status != 200 || r.json(t)["warranty_end"] != "2029-09-12" || r.json(t)["name"] != "ASUS TUF Gaming VG27AQ" {
		t.Fatalf("update %d %s", r.Status, r.Body)
	}
	delete(upd, "warranty_months")
	if r := h.do("PUT", "/api/v1/products/"+id, a, upd); r.json(t)["warranty_months"].(float64) != 36 {
		t.Fatal("months should be kept when omitted on update")
	}

	// reminders were planned (90/30/7)
	r = h.do("GET", "/api/v1/notifications", a, nil)
	ns := r.json(t)["items"].([]any)
	found := 0
	for _, n := range ns {
		m := n.(map[string]any)
		if m["product_id"] == id {
			found++
			if !strings.Contains(m["message"].(string), "ASUS monitörünüzün garantisinin bitmesine") {
				t.Fatalf("message %v", m["message"])
			}
		}
	}
	if found != 3 {
		t.Fatalf("expected 3 reminders, got %d (%s)", found, r.Body)
	}

	if r := h.do("DELETE", "/api/v1/products/"+id, a, nil); r.Status != 204 {
		t.Fatalf("delete %d", r.Status)
	}
	if r := h.do("GET", "/api/v1/products/"+id, a, nil); r.Status != 404 {
		t.Fatal("deleted product still visible")
	}
	if r := h.do("GET", "/api/v1/products/not-a-uuid", a, nil); r.Status != 404 {
		t.Fatal("bad id should be 404")
	}
}

func TestDocumentsUploadDownloadAndPack(t *testing.T) {
	h := newHarness(t)
	a := h.register("doc@ww.test", "Deniz")
	b := h.register("other@ww.test", "Other")
	id := h.do("POST", "/api/v1/products", a, monitor).json(t)["id"].(string)

	img := jpegBytes()
	r := h.do("POST", "/api/v1/products/"+id+"/documents", a, newMultipart(map[string]string{"type": "invoice"}, "file", "fatura.jpg", img))
	if r.Status != 201 {
		t.Fatalf("upload %d %s", r.Status, r.Body)
	}
	doc := r.json(t)
	if doc["content_type"] != "image/jpeg" || doc["type"] != "invoice" || doc["original_name"] != "fatura.jpg" {
		t.Fatalf("doc %v", doc)
	}
	docID := doc["id"].(string)

	// type sniffing: a "jpg" that is actually text is rejected
	r = h.do("POST", "/api/v1/products/"+id+"/documents", a, newMultipart(map[string]string{"type": "invoice"}, "file", "evil.jpg", []byte("#!/bin/sh\necho hi")))
	if r.Status != 415 {
		t.Fatalf("expected 415, got %d", r.Status)
	}
	// size limit
	big := append(jpegBytes(), make([]byte, 2<<20)...)
	if r := h.do("POST", "/api/v1/products/"+id+"/documents", a, newMultipart(nil, "file", "big.jpg", big)); r.Status != 413 {
		t.Fatalf("expected 413, got %d", r.Status)
	}
	// invalid type
	if r := h.do("POST", "/api/v1/products/"+id+"/documents", a, newMultipart(map[string]string{"type": "selfie"}, "file", "x.jpg", img)); r.Status != 422 {
		t.Fatalf("expected 422, got %d", r.Status)
	}
	// other user cannot upload to / list / download
	if r := h.do("POST", "/api/v1/products/"+id+"/documents", b, newMultipart(nil, "file", "x.jpg", img)); r.Status != 404 {
		t.Fatal("b uploaded to a's product")
	}
	if r := h.do("GET", "/api/v1/documents/"+docID+"/file", b, nil); r.Status != 404 {
		t.Fatal("b downloaded a's file")
	}

	// warranty card as PDF
	pdfData := []byte("%PDF-1.4\n%fake\n")
	if r := h.do("POST", "/api/v1/products/"+id+"/documents", a, newMultipart(map[string]string{"type": "warranty"}, "file", "garanti.pdf", pdfData)); r.Status != 201 {
		t.Fatalf("pdf upload %d %s", r.Status, r.Body)
	}
	r = h.do("GET", "/api/v1/products/"+id+"/documents", a, nil)
	if n := len(r.json(t)["items"].([]any)); n != 2 {
		t.Fatalf("docs %d", n)
	}
	if r := h.do("GET", "/api/v1/products/"+id+"/documents?type=warranty", a, nil); len(r.json(t)["items"].([]any)) != 1 {
		t.Fatal("type filter")
	}

	dl := h.do("GET", "/api/v1/documents/"+docID+"/file", a, nil)
	if dl.Status != 200 || !bytes.Equal(dl.Body, img) || dl.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("download %d %s", dl.Status, dl.Header.Get("Content-Type"))
	}

	// checklist: invoice + warranty + serial + model all present
	r = h.do("GET", "/api/v1/products/"+id+"/checklist", a, nil)
	cl := r.json(t)
	if cl["under_warranty"] != true {
		t.Fatal("should be under warranty")
	}
	for _, it := range cl["items"].([]any) {
		if it.(map[string]any)["ok"] != true {
			t.Fatalf("checklist item not ok: %v", it)
		}
	}

	// broken product flow → service record with checklist
	r = h.do("POST", "/api/v1/products/"+id+"/service-records", a, map[string]any{"problem": "screen_lines", "description": "Sol tarafta dikey çizgiler"})
	if r.Status != 201 {
		t.Fatalf("service %d %s", r.Status, r.Body)
	}
	sr := r.json(t)
	if sr["under_warranty"] != true || sr["status"] != "open" || len(sr["checklist"].([]any)) != 4 {
		t.Fatalf("service record %v", sr)
	}
	srID := sr["id"].(string)
	// photo attached to the service record
	if r := h.do("POST", "/api/v1/products/"+id+"/documents", a, newMultipart(map[string]string{"type": "photo", "service_record_id": srID}, "file", "ariza.jpg", img)); r.Status != 201 {
		t.Fatalf("service photo %d %s", r.Status, r.Body)
	}

	pack := h.do("GET", "/api/v1/products/"+id+"/service-pack.pdf?service_record_id="+srID, a, nil)
	if pack.Status != 200 || pack.Header.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(pack.Body, []byte("%PDF-1.4")) {
		t.Fatalf("pack %d %s", pack.Status, pack.Header.Get("Content-Type"))
	}
	if !bytes.Contains(pack.Body, []byte("/Count 3")) { // cover + invoice image + fault photo
		t.Fatal("pack should have 3 pages")
	}
	if r := h.do("GET", "/api/v1/products/"+id+"/service-pack.pdf", b, nil); r.Status != 404 {
		t.Fatal("b got a's pack")
	}

	if r := h.do("DELETE", "/api/v1/documents/"+docID, a, nil); r.Status != 204 {
		t.Fatalf("delete doc %d", r.Status)
	}
	if r := h.do("GET", "/api/v1/documents/"+docID+"/file", a, nil); r.Status != 404 {
		t.Fatal("deleted doc still downloadable")
	}
}

func TestServiceRecords(t *testing.T) {
	h := newHarness(t)
	a := h.register("svc@ww.test", "")
	id := h.do("POST", "/api/v1/products", a, monitor).json(t)["id"].(string)
	old := h.do("POST", "/api/v1/products", a, map[string]any{"name": "Eski Laptop", "category": "laptop", "purchase_date": "2019-05-01", "warranty_months": 24}).json(t)["id"].(string)

	if r := h.do("POST", "/api/v1/products/"+id+"/service-records", a, map[string]any{"problem": "exploded"}); r.Status != 422 {
		t.Fatalf("invalid problem %d", r.Status)
	}
	r := h.do("POST", "/api/v1/products/"+old+"/service-records", a, map[string]any{"problem": "wont_turn_on"})
	if r.Status != 201 || r.json(t)["under_warranty"] != false || r.json(t)["checklist"] != nil {
		t.Fatalf("expired product service: %s", r.Body)
	}
	srID := h.do("POST", "/api/v1/products/"+id+"/service-records", a, map[string]any{"problem": "physical_damage"}).json(t)["id"].(string)

	r = h.do("PATCH", "/api/v1/service-records/"+srID, a, map[string]any{"status": "in_service", "service_name": "ASUS Yetkili Servis Kadıköy", "service_date": "2026-10-17"})
	if r.Status != 200 || r.json(t)["status"] != "in_service" || r.json(t)["service_date"] != "2026-10-17" || r.json(t)["problem"] != "physical_damage" {
		t.Fatalf("patch %d %s", r.Status, r.Body)
	}
	if r := h.do("PATCH", "/api/v1/service-records/"+srID, a, map[string]any{"status": "lost"}); r.Status != 422 {
		t.Fatal("invalid status accepted")
	}
	r = h.do("GET", "/api/v1/service-records", a, nil)
	items := r.json(t)["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["product_name"] == "" {
		t.Fatalf("history %s", r.Body)
	}
	if r := h.do("GET", "/api/v1/service-records?status=in_service", a, nil); len(r.json(t)["items"].([]any)) != 1 {
		t.Fatal("status filter")
	}
	if r := h.do("GET", "/api/v1/products/"+id+"/service-records", a, nil); len(r.json(t)["items"].([]any)) != 1 {
		t.Fatal("product history")
	}
	other := h.register("svc2@ww.test", "")
	if r := h.do("PATCH", "/api/v1/service-records/"+srID, other, map[string]any{"status": "resolved"}); r.Status != 404 {
		t.Fatal("other user patched record")
	}
}

func TestExtractEndpoint(t *testing.T) {
	h := newHarness(t)
	a := h.register("ex@ww.test", "")
	text := "MediaMarkt\nTarih: 12.09.2026\nASUS TUF VG27AQ 27\" MONITOR 1 AD 12.499,00\nTOPLAM *12.499,00 TL\n24 AY GARANTI\n"
	r := h.do("POST", "/api/v1/extract", a, map[string]string{"text": text})
	if r.Status != 200 {
		t.Fatalf("extract %d %s", r.Status, r.Body)
	}
	m := r.json(t)
	f := m["fields"].(map[string]any)
	if m["provider"] != "rules" || f["seller"] != "MediaMarkt" || f["price"].(float64) != 12499 || m["warranty_end"] != "2028-09-12" {
		t.Fatalf("extract result %s", r.Body)
	}
	// image-only without AI → manual form
	r = h.do("POST", "/api/v1/extract", a, newMultipart(nil, "image", "fis.jpg", jpegBytes()))
	if r.Status != 200 || r.json(t)["provider"] != "none" || r.json(t)["suggested_warranty_months"].(float64) != 24 {
		t.Fatalf("image only %d %s", r.Status, r.Body)
	}
	// multipart with both image and OCR text
	r = h.do("POST", "/api/v1/extract", a, newMultipart(map[string]string{"text": text}, "image", "fis.jpg", jpegBytes()))
	if r.Status != 200 || r.json(t)["provider"] != "rules" {
		t.Fatalf("multipart %d %s", r.Status, r.Body)
	}
	if r := h.do("POST", "/api/v1/extract", a, map[string]string{"text": "   "}); r.Status != 422 {
		t.Fatalf("empty %d", r.Status)
	}
	if r := h.do("POST", "/api/v1/extract", "", map[string]string{"text": text}); r.Status != 401 {
		t.Fatal("extract requires auth")
	}
}

func TestDashboard(t *testing.T) {
	h := newHarness(t)
	a := h.register("dash@ww.test", "Berat Kaan")
	h.do("POST", "/api/v1/products", a, monitor)
	h.do("POST", "/api/v1/products", a, map[string]any{"name": "Sony WH-1000XM5", "brand": "Sony", "category": "headphones", "purchase_date": "2024-11-01", "price": 8999.9})
	h.do("POST", "/api/v1/products", a, map[string]any{"name": "Eski Telefon", "category": "phone", "purchase_date": "2020-01-01", "price": 5000})
	h.do("POST", "/api/v1/products", a, map[string]any{"name": "Kulaklık 2", "category": "headphones", "purchase_date": "2026-01-01", "price": 100.05})

	r := h.do("GET", "/api/v1/dashboard", a, nil)
	d := r.json(t)
	if d["total_products"].(float64) != 4 || d["active_warranties"].(float64) != 3 || d["expired"].(float64) != 1 || d["expiring_soon"].(float64) != 1 {
		t.Fatalf("dashboard %s", r.Body)
	}
	if d["total_spent"].(map[string]any)["TRY"].(float64) != 26598.95 {
		t.Fatalf("spent %v", d["total_spent"])
	}
	cats := d["by_category"].([]any)
	first := cats[0].(map[string]any)
	if first["category"] != "headphones" || first["count"].(float64) != 2 || first["name"] != "Kulaklık" {
		t.Fatalf("categories %v", cats)
	}
	if len(d["upcoming"].([]any)) != 1 || !strings.HasSuffix(d["greeting"].(string), ", Berat") {
		t.Fatalf("upcoming/greeting %v %v", d["upcoming"], d["greeting"])
	}
	if r := h.do("GET", "/api/v1/categories", a, nil); len(r.json(t)["items"].([]any)) != 12 {
		t.Fatal("categories")
	}
}

func TestAdmin(t *testing.T) {
	h := newHarness(t)
	admin := h.register("admin@ww.test", "Admin")
	user := h.register("u@ww.test", "User")

	if r := h.do("GET", "/api/v1/admin/users", user, nil); r.Status != 403 {
		t.Fatalf("non-admin got %d", r.Status)
	}
	r := h.do("GET", "/api/v1/admin/users", admin, nil)
	users := r.json(t)["items"].([]any)
	if len(users) != 2 {
		t.Fatalf("users %s", r.Body)
	}
	var uid, aid string
	for _, u := range users {
		m := u.(map[string]any)
		if m["email"] == "u@ww.test" {
			uid = m["id"].(string)
		} else {
			aid = m["id"].(string)
		}
	}
	if r := h.do("PATCH", "/api/v1/admin/users/"+aid, admin, map[string]string{"role": "user"}); r.Status != 422 {
		t.Fatal("admin demoted self")
	}
	if r := h.do("PATCH", "/api/v1/admin/users/"+uid, admin, map[string]string{"role": "admin"}); r.Status != 200 || r.json(t)["role"] != "admin" {
		t.Fatalf("promote %s", r.Body)
	}
	// categories
	if r := h.do("POST", "/api/v1/admin/categories", admin, map[string]string{"slug": "drone", "name_tr": "Drone"}); r.Status != 200 {
		t.Fatalf("category %d %s", r.Status, r.Body)
	}
	if r := h.do("POST", "/api/v1/admin/categories", admin, map[string]string{"slug": "Bad Slug", "name_tr": ""}); r.Status != 422 {
		t.Fatal("bad category accepted")
	}
	if r := h.do("POST", "/api/v1/products", user, map[string]any{"name": "DJI Mini", "category": "drone", "purchase_date": "2026-05-01"}); r.Status != 201 {
		t.Fatalf("new category not usable: %s", r.Body)
	}
	if r := h.do("DELETE", "/api/v1/admin/categories/other", admin, nil); r.Status != 422 {
		t.Fatal("deleted 'other'")
	}
	if r := h.do("DELETE", "/api/v1/admin/categories/drone", admin, nil); r.Status != 204 {
		t.Fatal("delete category")
	}
	// extraction logs (OCR errors view)
	h.do("POST", "/api/v1/extract", user, map[string]string{"text": "okunaksız fiş"})
	r = h.do("GET", "/api/v1/admin/extraction-logs?failures=1", admin, nil)
	if len(r.json(t)["items"].([]any)) != 1 {
		t.Fatalf("logs %s", r.Body)
	}
	r = h.do("GET", "/api/v1/admin/stats", admin, nil)
	if r.json(t)["users"].(float64) != 2 || r.json(t)["products"].(float64) != 1 {
		t.Fatalf("stats %s", r.Body)
	}
	// delete user
	if r := h.do("DELETE", "/api/v1/admin/users/"+uid, admin, nil); r.Status != 204 {
		t.Fatal("delete user")
	}
	if r := h.do("GET", "/api/v1/me", user, nil); r.Status != 401 {
		t.Fatal("deleted user's token still works")
	}
}

func TestDeleteMe(t *testing.T) {
	h := newHarness(t)
	a := h.register("bye@ww.test", "")
	id := h.do("POST", "/api/v1/products", a, monitor).json(t)["id"].(string)
	h.do("POST", "/api/v1/products/"+id+"/documents", a, newMultipart(nil, "file", "f.jpg", jpegBytes()))
	if r := h.do("DELETE", "/api/v1/me", a, nil); r.Status != 204 {
		t.Fatal("delete me")
	}
	if r := h.do("POST", "/api/v1/auth/login", "", map[string]string{"email": "bye@ww.test", "password": "supersecret"}); r.Status != 401 {
		t.Fatal("account still exists")
	}
}

func TestCORSPreflight(t *testing.T) {
	h := newHarness(t)
	req, _ := http.NewRequest("OPTIONS", h.srv.URL+"/api/v1/products", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 204 || res.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("cors %d %v", res.StatusCode, res.Header)
	}
}
