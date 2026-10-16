package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

// APIError is the uniform error body: {"error": {...}}.
type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string, fields map[string]string) {
	writeJSON(w, status, map[string]APIError{"error": {Code: code, Message: msg, Fields: fields}})
}

func (s *Server) storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found", nil)
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "resource already exists", nil)
	default:
		s.Logger.Error("store error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error", nil)
	}
}

// decodeJSON reads a ≤1MB JSON body, rejecting unknown fields to catch client typos.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		msg := "invalid JSON body"
		if !errors.Is(err, io.EOF) {
			msg += ": " + err.Error()
		}
		writeError(w, http.StatusBadRequest, "bad_request", msg, nil)
		return false
	}
	return true
}

// clientIP returns the caller's address. X-Forwarded-For is client-controlled, so it is only
// honoured when the API runs behind a trusted reverse proxy (TRUST_PROXY=true); otherwise a
// client could rotate the header to bypass rate limits.
func clientIP(r *http.Request, trustProxy bool) string {
	if xf := r.Header.Get("X-Forwarded-For"); trustProxy && xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimiter is a fixed-window per-IP limiter (good enough for a single instance).
type rateLimiter struct {
	mu         sync.Mutex
	limit      int
	window     time.Duration
	trustProxy bool
	hits       map[string]*windowCount
}

type windowCount struct {
	start time.Time
	n     int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, hits: map[string]*windowCount{}}
}

func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) > 10000 { // bound memory
		for k, v := range l.hits {
			if now.Sub(v.start) > l.window {
				delete(l.hits, k)
			}
		}
	}
	c, ok := l.hits[key]
	if !ok || now.Sub(c.start) > l.window {
		l.hits[key] = &windowCount{start: now, n: 1}
		return true
	}
	c.n++
	return c.n <= l.limit
}

func (l *rateLimiter) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r, l.trustProxy), time.Now()) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests, try again later", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
