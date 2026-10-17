package httpapi

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestAdminPanelServedWithCSP(t *testing.T) {
	h := newHarness(t)
	r := h.do("GET", "/admin/", "", nil)
	if r.Status != 200 || !strings.Contains(string(r.Body), "Warranty Wallet · Yönetim") {
		t.Fatalf("index %d", r.Status)
	}
	csp := r.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") || strings.Contains(csp, "unsafe-inline") {
		t.Errorf("csp %q", csp)
	}
	if r := h.do("GET", "/admin/admin.js", "", nil); r.Status != 200 || !strings.Contains(r.Header.Get("Content-Type"), "javascript") {
		t.Errorf("admin.js %d %s", r.Status, r.Header.Get("Content-Type"))
	}
	if r := h.do("GET", "/admin/../go.mod", "", nil); r.Status == 200 && strings.Contains(string(r.Body), "module ") {
		t.Error("path traversal served a source file")
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Get(h.srv.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != "/admin/" {
		t.Errorf("redirect %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	// The panel must not get any data without an admin token.
	if r := h.do("GET", "/api/v1/admin/stats", h.register("plain@example.com", "P"), nil); r.Status != 403 {
		t.Errorf("non-admin stats %d", r.Status)
	}
}

// The panel renders server data with textContent only; innerHTML would allow stored XSS
// through user names or OCR error texts.
func TestAdminPanelAvoidsInnerHTML(t *testing.T) {
	js, err := os.ReadFile("../adminui/static/admin.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "localStorage"} {
		if strings.Contains(string(js), bad) {
			t.Errorf("admin.js uses %s", bad)
		}
	}
}
