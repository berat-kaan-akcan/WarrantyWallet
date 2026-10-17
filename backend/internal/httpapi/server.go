// Package httpapi exposes the REST API (/api/v1) over net/http.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/adminui"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/assist"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/auth"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/extract"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/mail"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/storage"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store"
)

// Server wires dependencies for the HTTP handlers.
type Server struct {
	Store      store.Store
	Files      storage.Storage
	Extractor  extract.ReceiptExtractor
	Assistant  assist.Assistant // AI fault assistant; zero value = rules only
	Tokens     auth.Tokens
	RefreshTTL time.Duration // refresh token lifetime (default 30 days)
	Mailer     mail.Sender
	// PublicBaseURL is the externally reachable API origin used in e-mail links.
	PublicBaseURL string
	TrustProxy    bool // honour X-Forwarded-For (only behind a trusted reverse proxy)
	Hasher        auth.Hasher
	Logger        *slog.Logger
	MaxUpload     int64 // bytes
	MaxImport     int64 // backup archive bytes (default DefaultMaxImport)
	AdminEmails   []string
	CORSOrigins   string
	Now           func() time.Time
	Version       string

	authLimiter    *rateLimiter
	extractLimiter *rateLimiter
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// today is the user's calendar day (Türkiye, UTC+3).
func (s *Server) today() domain.Date {
	return domain.DateOf(s.now().In(time.FixedZone("TRT", 3*3600)))
}

// Handler builds the routed, middleware-wrapped handler.
func (s *Server) Handler() http.Handler {
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	if s.MaxUpload <= 0 {
		s.MaxUpload = 10 << 20
	}
	if s.authLimiter == nil {
		s.authLimiter = newRateLimiter(10, time.Minute) // 10 auth attempts / minute / IP
	}
	s.authLimiter.trustProxy = s.TrustProxy
	if s.extractLimiter == nil {
		s.extractLimiter = newRateLimiter(20, time.Minute)
	}
	s.extractLimiter.trustProxy = s.TrustProxy
	mux := http.NewServeMux()
	a := s.requireAuth
	admin := func(h http.HandlerFunc) http.Handler { return s.requireAuth(s.requireAdmin(h)) }

	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/v1/healthz", s.handleHealth)

	mux.Handle("POST /api/v1/auth/register", s.authLimiter.wrap(http.HandlerFunc(s.handleRegister)))
	mux.Handle("POST /api/v1/auth/login", s.authLimiter.wrap(http.HandlerFunc(s.handleLogin)))
	mux.Handle("POST /api/v1/auth/refresh", s.authLimiter.wrap(http.HandlerFunc(s.handleRefresh)))
	mux.Handle("POST /api/v1/auth/logout", a(http.HandlerFunc(s.handleLogout)))
	mux.Handle("POST /api/v1/auth/password/forgot", s.authLimiter.wrap(http.HandlerFunc(s.handleForgotPassword)))
	mux.Handle("POST /api/v1/auth/password/reset", s.authLimiter.wrap(http.HandlerFunc(s.handleResetPassword)))
	mux.Handle("POST /api/v1/auth/email/resend", a(s.authLimiter.wrap(http.HandlerFunc(s.handleResendVerification))))
	mux.Handle("POST /api/v1/auth/email/verify", s.authLimiter.wrap(http.HandlerFunc(s.handleVerifyEmailJSON)))
	mux.Handle("GET /api/v1/auth/email/verify", s.authLimiter.wrap(http.HandlerFunc(s.handleVerifyEmailLink)))
	mux.Handle("GET /api/v1/me", a(http.HandlerFunc(s.handleMe)))
	mux.Handle("PATCH /api/v1/me", a(http.HandlerFunc(s.handleUpdateMe)))
	mux.Handle("PUT /api/v1/me/password", a(http.HandlerFunc(s.handleChangePassword)))
	mux.Handle("DELETE /api/v1/me", a(http.HandlerFunc(s.handleDeleteMe)))
	mux.Handle("GET /api/v1/me/export", a(http.HandlerFunc(s.handleExport)))
	mux.Handle("POST /api/v1/me/import", a(s.extractLimiter.wrap(http.HandlerFunc(s.handleImport))))

	mux.Handle("GET /api/v1/categories", a(http.HandlerFunc(s.handleCategories)))

	mux.Handle("GET /api/v1/products", a(http.HandlerFunc(s.handleListProducts)))
	mux.Handle("POST /api/v1/products", a(http.HandlerFunc(s.handleCreateProduct)))
	mux.Handle("GET /api/v1/products/expiring", a(http.HandlerFunc(s.handleExpiring)))
	mux.Handle("GET /api/v1/products/{id}", a(http.HandlerFunc(s.handleGetProduct)))
	mux.Handle("PUT /api/v1/products/{id}", a(http.HandlerFunc(s.handleUpdateProduct)))
	mux.Handle("DELETE /api/v1/products/{id}", a(http.HandlerFunc(s.handleDeleteProduct)))
	mux.Handle("GET /api/v1/products/{id}/service-centers", a(http.HandlerFunc(s.handleProductServiceCenters)))
	mux.Handle("GET /api/v1/service-centers", a(http.HandlerFunc(s.handleServiceCenters)))
	mux.Handle("GET /api/v1/products/{id}/checklist", a(http.HandlerFunc(s.handleChecklist)))
	mux.Handle("GET /api/v1/products/{id}/service-pack.pdf", a(http.HandlerFunc(s.handleServicePack)))

	mux.Handle("POST /api/v1/products/{id}/diagnose", a(s.extractLimiter.wrap(http.HandlerFunc(s.handleDiagnose))))

	mux.Handle("POST /api/v1/products/{id}/documents", a(http.HandlerFunc(s.handleUploadDocument)))
	mux.Handle("GET /api/v1/products/{id}/documents", a(http.HandlerFunc(s.handleListDocuments)))
	mux.Handle("GET /api/v1/documents/{id}/file", a(http.HandlerFunc(s.handleDownloadDocument)))
	mux.Handle("DELETE /api/v1/documents/{id}", a(http.HandlerFunc(s.handleDeleteDocument)))

	mux.Handle("POST /api/v1/products/{id}/service-records", a(http.HandlerFunc(s.handleCreateServiceRecord)))
	mux.Handle("GET /api/v1/products/{id}/service-records", a(http.HandlerFunc(s.handleListProductServiceRecords)))
	mux.Handle("GET /api/v1/service-records", a(http.HandlerFunc(s.handleListServiceRecords)))
	mux.Handle("GET /api/v1/service-records/{id}", a(http.HandlerFunc(s.handleGetServiceRecord)))
	mux.Handle("PATCH /api/v1/service-records/{id}", a(http.HandlerFunc(s.handleUpdateServiceRecord)))
	mux.Handle("DELETE /api/v1/service-records/{id}", a(http.HandlerFunc(s.handleDeleteServiceRecord)))

	mux.Handle("POST /api/v1/extract", a(s.extractLimiter.wrap(http.HandlerFunc(s.handleExtract))))
	mux.Handle("GET /api/v1/dashboard", a(http.HandlerFunc(s.handleDashboard)))
	mux.Handle("GET /api/v1/notifications", a(http.HandlerFunc(s.handleNotifications)))
	mux.Handle("GET /api/v1/family", a(http.HandlerFunc(s.handleGetFamily)))
	mux.Handle("POST /api/v1/family", a(http.HandlerFunc(s.handleCreateFamily)))
	mux.Handle("PATCH /api/v1/family", a(http.HandlerFunc(s.handleRenameFamily)))
	mux.Handle("POST /api/v1/family/invites", a(http.HandlerFunc(s.handleCreateInvite)))
	mux.Handle("POST /api/v1/family/join", a(s.authLimiter.wrap(http.HandlerFunc(s.handleJoinFamily))))
	mux.Handle("DELETE /api/v1/family/members/{id}", a(http.HandlerFunc(s.handleRemoveFamilyMember)))
	mux.Handle("GET /api/v1/family/products", a(http.HandlerFunc(s.handleFamilyProducts)))
	mux.Handle("POST /api/v1/devices", a(http.HandlerFunc(s.handleRegisterDevice)))
	mux.Handle("DELETE /api/v1/devices/{token}", a(http.HandlerFunc(s.handleUnregisterDevice)))

	mux.Handle("GET /api/v1/admin/stats", admin(s.handleAdminStats))
	mux.Handle("GET /api/v1/admin/users", admin(s.handleAdminUsers))
	mux.Handle("PATCH /api/v1/admin/users/{id}", admin(s.handleAdminUpdateUser))
	mux.Handle("DELETE /api/v1/admin/users/{id}", admin(s.handleAdminDeleteUser))
	mux.Handle("GET /api/v1/admin/extraction-logs", admin(s.handleAdminExtractionLogs))
	mux.Handle("POST /api/v1/admin/categories", admin(s.handleAdminUpsertCategory))
	mux.Handle("DELETE /api/v1/admin/categories/{slug}", admin(s.handleAdminDeleteCategory))
	mux.Handle("GET /api/v1/admin/service-centers", admin(s.handleAdminServiceCenters))
	mux.Handle("POST /api/v1/admin/service-centers", admin(s.handleAdminCreateServiceCenter))
	mux.Handle("PUT /api/v1/admin/service-centers/{id}", admin(s.handleAdminUpdateServiceCenter))
	mux.Handle("DELETE /api/v1/admin/service-centers/{id}", admin(s.handleAdminDeleteServiceCenter))

	// Web admin panel (static; every action goes through the admin API above).
	mux.Handle("GET /admin/", adminui.Handler())
	mux.Handle("GET /admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
	})
	return s.recoverer(s.logRequests(s.cors(securityHeaders(mux))))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	status, code := "ok", http.StatusOK
	if err := s.Store.Ping(ctx); err != nil {
		status, code = "degraded", http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{"status": status, "version": s.Version, "extractor": extractorName(s.Extractor)})
}

func extractorName(e extract.ReceiptExtractor) string {
	if e == nil {
		return "none"
	}
	if fb, ok := e.(*extract.Fallback); ok && fb.Primary != nil {
		if g, ok := fb.Primary.(*extract.GeminiExtractor); ok && g.APIKey != "" {
			return "gemini+rules"
		}
		return "rules"
	}
	return e.Name()
}

// ---- middleware ----

type ctxKey int

const (
	ctxUser ctxKey = iota
)

// userFrom returns the authenticated user.
func userFrom(ctx context.Context) domain.User {
	u, _ := ctx.Value(ctxUser).(domain.User)
	return u
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		tok, ok := strings.CutPrefix(h, "Bearer ")
		if !ok || tok == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token", nil)
			return
		}
		claims, err := s.Tokens.Verify(strings.TrimSpace(tok))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token", nil)
			return
		}
		u, err := s.Store.GetUserByID(r.Context(), claims.Subject)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "user no longer exists", nil)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUser, u)))
	})
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userFrom(r.Context()).Role != domain.RoleAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "admin role required", nil)
			return
		}
		next(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		if r.URL.Path == "/healthz" {
			return
		}
		// Only method, path and status are logged: never headers or bodies (tokens, receipts).
		s.Logger.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "dur_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.Logger.Error("panic", "path", r.URL.Path, "panic", v)
				writeError(w, http.StatusInternalServerError, "internal", "internal server error", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := strings.Split(s.CORSOrigins, ",")
	for i := range allowed {
		allowed[i] = strings.TrimSpace(allowed[i])
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (slices.Contains(allowed, "*") || slices.Contains(allowed, origin)) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
