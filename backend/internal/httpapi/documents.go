package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/storage"
)

var allowedTypes = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/heic": ".heic", "application/pdf": ".pdf",
}

// sniffContentType trusts magic bytes, not the client-declared type. HEIC is not detected
// by the stdlib, so it is recognised via its ISO-BMFF "ftypheic/heix/mif1" brand.
func sniffContentType(head []byte) string {
	ct := http.DetectContentType(head)
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	if ct == "application/octet-stream" && len(head) >= 12 && string(head[4:8]) == "ftyp" {
		switch string(head[8:12]) {
		case "heic", "heix", "mif1", "msf1", "hevc":
			return "image/heic"
		}
	}
	return ct
}

// readUpload reads a multipart file part with size and type validation.
func (s *Server) readUpload(w http.ResponseWriter, r *http.Request, field string, required bool) (data []byte, ct, name string, ok bool) {
	file, hdr, err := r.FormFile(field)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) && !required {
			return nil, "", "", true
		}
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("file exceeds %d MB", s.MaxUpload>>20), nil)
			return nil, "", "", false
		}
		writeError(w, http.StatusBadRequest, "bad_request", "multipart field '"+field+"' is required", nil)
		return nil, "", "", false
	}
	defer file.Close()
	data, err = io.ReadAll(io.LimitReader(file, s.MaxUpload+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "could not read upload", nil)
		return nil, "", "", false
	}
	if int64(len(data)) > s.MaxUpload {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("file exceeds %d MB", s.MaxUpload>>20), nil)
		return nil, "", "", false
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "empty file", nil)
		return nil, "", "", false
	}
	ct = sniffContentType(data[:min(len(data), 512)])
	if _, okType := allowedTypes[ct]; !okType {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "allowed: JPEG, PNG, WEBP, HEIC, PDF", map[string]string{field: ct})
		return nil, "", "", false
	}
	name = path.Base(strings.ReplaceAll(hdr.Filename, `\`, "/"))
	if name == "." || name == "/" || name == "" {
		name = "belge" + allowedTypes[ct]
	}
	if len(name) > 120 {
		name = name[len(name)-120:]
	}
	return data, ct, name, true
}

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	p, err := s.Store.GetProduct(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.MaxUpload+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("file exceeds %d MB", s.MaxUpload>>20), nil)
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", "expected multipart/form-data", nil)
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	typ := domain.DocumentType(r.FormValue("type"))
	if typ == "" {
		typ = domain.DocInvoice
	}
	if !domain.ValidDocumentType(typ) {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid document type", map[string]string{"type": "invoice|warranty|service|photo"})
		return
	}
	srID := r.FormValue("service_record_id")
	if srID != "" {
		sr, err := s.Store.GetServiceRecord(r.Context(), u.ID, srID)
		if err != nil || sr.ProductID != p.ID {
			writeError(w, http.StatusUnprocessableEntity, "validation_failed", "unknown service record", map[string]string{"service_record_id": "not found for this product"})
			return
		}
	}
	data, ct, name, ok := s.readUpload(w, r, "file", true)
	if !ok {
		return
	}
	doc := domain.Document{ID: domain.NewID(), UserID: u.ID, ProductID: p.ID, ServiceRecordID: srID, Type: typ,
		ContentType: ct, SizeBytes: int64(len(data)), OriginalName: name}
	doc.FileKey = fmt.Sprintf("%s/%s/%s%s", u.ID, p.ID, doc.ID, allowedTypes[ct])
	if _, err := s.Files.Put(r.Context(), doc.FileKey, bytes.NewReader(data)); err != nil {
		s.storeError(w, err)
		return
	}
	if err := s.Store.CreateDocument(r.Context(), &doc); err != nil {
		_ = s.Files.Delete(r.Context(), doc.FileKey)
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	if _, err := s.Store.GetProduct(r.Context(), u.ID, r.PathValue("id")); err != nil {
		s.storeError(w, err)
		return
	}
	docs, err := s.Store.ListDocuments(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	if t := r.URL.Query().Get("type"); t != "" {
		filtered := docs[:0]
		for _, d := range docs {
			if string(d.Type) == t {
				filtered = append(filtered, d)
			}
		}
		docs = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": docs})
}

func (s *Server) handleDownloadDocument(w http.ResponseWriter, r *http.Request) {
	d, err := s.Store.GetDocument(r.Context(), userFrom(r.Context()).ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	rc, err := s.Files.Open(r.Context(), d.FileKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "file missing from storage", nil)
			return
		}
		s.storeError(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", d.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(d.SizeBytes, 10))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": d.OriginalName}))
	_, _ = io.Copy(w, rc)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	d, err := s.Store.GetDocument(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	if err := s.Store.DeleteDocument(r.Context(), u.ID, d.ID); err != nil {
		s.storeError(w, err)
		return
	}
	if err := s.Files.Delete(r.Context(), d.FileKey); err != nil {
		s.Logger.Warn("file delete failed", "key", d.FileKey, "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}
