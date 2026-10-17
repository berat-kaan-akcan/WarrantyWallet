package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func (h *harness) postZip(token string, data []byte) resp {
	h.t.Helper()
	req, _ := http.NewRequest("POST", h.srv.URL+"/api/v1/me/import", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{res.StatusCode, b, res.Header}
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write(data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestBackupExportImportRoundTrip(t *testing.T) {
	h := newHarness(t)
	a := h.register("backup@ww.test", "Ayşe")
	pid := h.do("POST", "/api/v1/products", a, monitor).json(t)["id"].(string)
	sr := h.do("POST", "/api/v1/products/"+pid+"/service-records", a, map[string]any{
		"problem": "screen_lines", "description": "Ekranda çizgi", "service_name": "ASUS Servis", "event_note": "Kayıt açıldı"}).json(t)
	srID := sr["id"].(string)
	if r := h.do("PATCH", "/api/v1/service-records/"+srID, a, map[string]any{"status": "resolved", "cost": 350, "event_note": "Panel değişti"}); r.Status != 200 {
		t.Fatalf("patch %d %s", r.Status, r.Body)
	}
	img := jpegBytes()
	if r := h.do("POST", "/api/v1/products/"+pid+"/documents", a, newMultipart(map[string]string{"type": "service", "service_record_id": srID}, "file", "servis.jpg", img)); r.Status != 201 {
		t.Fatalf("upload %d", r.Status)
	}

	ex := h.do("GET", "/api/v1/me/export", a, nil)
	if ex.Status != 200 || ex.Header.Get("Content-Type") != "application/zip" ||
		!strings.Contains(ex.Header.Get("Content-Disposition"), "warranty-wallet-yedek-") {
		t.Fatalf("export %d %v", ex.Status, ex.Header)
	}
	zr, err := zip.NewReader(bytes.NewReader(ex.Body), int64(len(ex.Body)))
	if err != nil {
		t.Fatal(err)
	}
	var manifest backupFile
	for _, f := range zr.File {
		if f.Name == backupManifest {
			if err := readZipJSON(f, &manifest); err != nil {
				t.Fatal(err)
			}
		}
	}
	if manifest.Format != backupFormat || len(manifest.Products) != 1 || len(manifest.Products[0].Documents) != 1 ||
		len(manifest.Products[0].ServiceRecords[0].Events) != 2 || manifest.Account.Email != "backup@ww.test" {
		js, _ := json.Marshal(manifest)
		t.Fatalf("manifest %s", js)
	}
	if strings.Contains(string(ex.Body), "password") {
		t.Fatal("backup must not contain credentials")
	}

	// restore into a fresh account
	b := h.register("restore@ww.test", "Ayşe")
	r := h.postZip(b, ex.Body)
	if r.Status != 200 {
		t.Fatalf("import %d %s", r.Status, r.Body)
	}
	got := r.json(t)
	if got["products"] != 1.0 || got["service_records"] != 1.0 || got["documents"] != 1.0 || got["failed"] != 0.0 {
		t.Fatalf("import result %v", got)
	}
	list := h.do("GET", "/api/v1/products", b, nil).json(t)["items"].([]any)
	np := list[0].(map[string]any)
	if np["name"] != "ASUS TUF VG27AQ" || np["warranty_end"] != "2028-09-12" || np["id"] == pid {
		t.Fatalf("restored product %v", np)
	}
	recs := h.do("GET", "/api/v1/service-records", b, nil).json(t)["items"].([]any)
	rec := h.do("GET", "/api/v1/service-records/"+recs[0].(map[string]any)["id"].(string), b, nil).json(t)
	if rec["status"] != "resolved" || rec["cost"] != 350.0 || rec["resolved_at"] == nil || len(rec["events"].([]any)) != 2 {
		t.Fatalf("restored record %v", rec)
	}
	docs := h.do("GET", "/api/v1/products/"+np["id"].(string)+"/documents", b, nil).json(t)["items"].([]any)
	d := docs[0].(map[string]any)
	if d["service_record_id"] != rec["id"] || d["original_name"] != "servis.jpg" {
		t.Fatalf("restored doc %v", d)
	}
	if f := h.do("GET", "/api/v1/documents/"+d["id"].(string)+"/file", b, nil); f.Status != 200 || !bytes.Equal(f.Body, img) {
		t.Fatalf("restored file %d", f.Status)
	}
	if n := h.do("GET", "/api/v1/notifications", b, nil).json(t)["items"].([]any); len(n) == 0 {
		t.Fatal("reminders not planned for restored product")
	}

	// importing again creates no duplicates
	again := h.postZip(b, ex.Body).json(t)
	if again["products"] != 0.0 || again["skipped_duplicates"] != 1.0 {
		t.Fatalf("second import %v", again)
	}
}

func TestBackupImportRejectsBadArchives(t *testing.T) {
	h := newHarness(t)
	a := h.register("bad@ww.test", "B")
	for name, body := range map[string][]byte{
		"not zip":     []byte("hello"),
		"no manifest": zipOf(t, map[string][]byte{"x.txt": []byte("x")}),
		"wrong fmt":   zipOf(t, map[string][]byte{"backup.json": []byte(`{"format":"other","version":1}`)}),
		"future":      zipOf(t, map[string][]byte{"backup.json": []byte(`{"format":"warranty-wallet-backup","version":9}`)}),
	} {
		if r := h.postZip(a, body); r.Status != 422 || !strings.Contains(string(r.Body), "invalid_backup") {
			t.Fatalf("%s: %d %s", name, r.Status, r.Body)
		}
	}
	// a bad product and a disguised file are reported, the rest is imported
	manifest := `{"format":"warranty-wallet-backup","version":1,"products":[
	  {"ref":"1","name":"","category":"monitor","purchase_date":"2026-01-01","currency":"TRY"},
	  {"ref":"2","name":"Kettle","category":"small_appliance","purchase_date":"2026-01-01","currency":"TRY","warranty_months":24,
	   "documents":[{"ref":"d1","type":"invoice","original_name":"evil.jpg","file":"files/d1.jpg"},
	                {"ref":"d2","type":"invoice","original_name":"yok.pdf","file":"files/none.pdf"}]}]}`
	r := h.postZip(a, zipOf(t, map[string][]byte{"backup.json": []byte(manifest), "files/d1.jpg": []byte("#!/bin/sh\necho hi")}))
	got := r.json(t)
	if r.Status != 200 || got["products"] != 1.0 || got["documents"] != 0.0 || got["failed"] != 3.0 || len(got["errors"].([]any)) != 3 {
		t.Fatalf("partial import %d %v", r.Status, got)
	}
	h.api.MaxImport = 100
	if r := h.postZip(a, bytes.Repeat([]byte("x"), 200)); r.Status != 413 {
		t.Fatalf("size limit %d", r.Status)
	}
	if r := h.do("GET", "/api/v1/me/export", "", nil); r.Status != 401 {
		t.Fatalf("export without auth %d", r.Status)
	}
}
