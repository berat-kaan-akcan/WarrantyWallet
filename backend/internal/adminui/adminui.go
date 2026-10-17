// Package adminui serves the lightweight web admin panel (V2) at /admin/.
// It is plain HTML/JS embedded in the binary and talks only to the /api/v1/admin endpoints,
// so authorization stays on the API (requireAuth + requireAdmin).
package adminui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var files embed.FS

// csp forbids inline script, third-party origins and framing.
const csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

// Handler serves the panel; mount it at /admin/.
func Handler() http.Handler {
	sub, _ := fs.Sub(files, "static")
	fsrv := http.StripPrefix("/admin/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Cache-Control", "no-store")
		fsrv.ServeHTTP(w, r)
	})
}
