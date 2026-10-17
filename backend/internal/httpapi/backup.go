package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

// Backup archive: a ZIP with backup.json plus every document under files/. It holds everything
// the user owns (products, service history, documents) so it can be restored into any account.
const (
	backupFormat      = "warranty-wallet-backup"
	backupVersion     = 1
	backupManifest    = "backup.json"
	maxBackupProducts = 5000
	maxManifestBytes  = 20 << 20
	// DefaultMaxImport caps an uploaded backup archive.
	DefaultMaxImport = 200 << 20
)

type backupFile struct {
	Format     string          `json:"format"`
	Version    int             `json:"version"`
	ExportedAt time.Time       `json:"exported_at"`
	Account    backupAccount   `json:"account"`
	Products   []backupProduct `json:"products"`
}

type backupAccount struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type backupProduct struct {
	Ref string `json:"ref"`
	productInput
	CreatedAt      time.Time        `json:"created_at"`
	ServiceRecords []backupService  `json:"service_records"`
	Documents      []backupDocument `json:"documents"`
}

type backupService struct {
	Ref           string               `json:"ref"`
	Problem       domain.ProblemType   `json:"problem"`
	Description   string               `json:"description"`
	ServiceName   string               `json:"service_name"`
	ServiceDate   domain.Date          `json:"service_date"`
	Status        domain.ServiceStatus `json:"status"`
	Notes         string               `json:"notes"`
	UnderWarranty bool                 `json:"under_warranty"`
	Cost          float64              `json:"cost"`
	ResolvedAt    *time.Time           `json:"resolved_at,omitempty"`
	CreatedAt     time.Time            `json:"created_at"`
	Events        []backupEvent        `json:"events"`
}

type backupEvent struct {
	Status    domain.ServiceStatus `json:"status"`
	Note      string               `json:"note"`
	CreatedAt time.Time            `json:"created_at"`
}

type backupDocument struct {
	Ref              string              `json:"ref"`
	Type             domain.DocumentType `json:"type"`
	ServiceRecordRef string              `json:"service_record_ref,omitempty"`
	ContentType      string              `json:"content_type"`
	OriginalName     string              `json:"original_name"`
	File             string              `json:"file"` // path inside the archive
	CreatedAt        time.Time           `json:"created_at"`
}

// handleExport streams the user's backup archive.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	products, err := s.Store.ListProducts(ctx, u.ID, storeFilterAll)
	if err != nil {
		s.storeError(w, err)
		return
	}
	bf := backupFile{Format: backupFormat, Version: backupVersion, ExportedAt: s.now().UTC(),
		Account: backupAccount{Email: u.Email, Name: u.Name}, Products: []backupProduct{}}
	var docs []domain.Document
	for _, p := range products {
		months := p.WarrantyMonths
		bp := backupProduct{Ref: p.ID, CreatedAt: p.CreatedAt, ServiceRecords: []backupService{}, Documents: []backupDocument{},
			productInput: productInput{Name: p.Name, Brand: p.Brand, Model: p.Model, Category: p.Category, Seller: p.Seller,
				PurchaseDate: p.PurchaseDate, Price: p.Price, Currency: p.Currency, WarrantyMonths: &months,
				SerialNumber: p.SerialNumber, Barcode: p.Barcode, Notes: p.Notes}}
		recs, err := s.Store.ListServiceRecords(ctx, u.ID, p.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		for _, rec := range recs {
			evs, err := s.Store.ListServiceEvents(ctx, u.ID, rec.ID)
			if err != nil {
				s.storeError(w, err)
				return
			}
			bs := backupService{Ref: rec.ID, Problem: rec.Problem, Description: rec.Description, ServiceName: rec.ServiceName,
				ServiceDate: rec.ServiceDate, Status: rec.Status, Notes: rec.Notes, UnderWarranty: rec.UnderWarranty,
				Cost: rec.Cost, ResolvedAt: rec.ResolvedAt, CreatedAt: rec.CreatedAt, Events: []backupEvent{}}
			for _, e := range evs {
				bs.Events = append(bs.Events, backupEvent{Status: e.Status, Note: e.Note, CreatedAt: e.CreatedAt})
			}
			bp.ServiceRecords = append(bp.ServiceRecords, bs)
		}
		pdocs, err := s.Store.ListDocuments(ctx, u.ID, p.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		for _, d := range pdocs {
			bp.Documents = append(bp.Documents, backupDocument{Ref: d.ID, Type: d.Type, ServiceRecordRef: d.ServiceRecordID,
				ContentType: d.ContentType, OriginalName: d.OriginalName, File: "files/" + d.ID + allowedTypes[d.ContentType],
				CreatedAt: d.CreatedAt})
		}
		docs = append(docs, pdocs...)
		bf.Products = append(bf.Products, bp)
	}

	name := "warranty-wallet-yedek-" + s.today().String() + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	zw := zip.NewWriter(w)
	missing := map[string]bool{}
	for _, d := range docs {
		if err := s.copyToZip(r, zw, d); err != nil {
			s.Logger.Warn("backup: document skipped", "document", d.ID, "err", err)
			missing[d.ID] = true
		}
	}
	if len(missing) > 0 { // keep the manifest consistent with the archive
		for i := range bf.Products {
			kept := bf.Products[i].Documents[:0]
			for _, d := range bf.Products[i].Documents {
				if !missing[d.Ref] {
					kept = append(kept, d)
				}
			}
			bf.Products[i].Documents = kept
		}
	}
	mw, err := zw.CreateHeader(&zip.FileHeader{Name: backupManifest, Method: zip.Deflate, Modified: bf.ExportedAt})
	if err == nil {
		enc := json.NewEncoder(mw)
		enc.SetIndent("", "  ")
		err = enc.Encode(bf)
	}
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		s.Logger.Error("backup export", "err", err) // headers are already sent
	}
}

func (s *Server) copyToZip(r *http.Request, zw *zip.Writer, d domain.Document) error {
	rc, err := s.Files.Open(r.Context(), d.FileKey)
	if err != nil {
		return err
	}
	defer rc.Close()
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: "files/" + d.ID + allowedTypes[d.ContentType], Method: zip.Store, Modified: d.CreatedAt})
	if err != nil {
		return err
	}
	_, err = io.Copy(fw, rc)
	return err
}

// importResult reports what a restore did.
type importResult struct {
	Products          int      `json:"products"`
	ServiceRecords    int      `json:"service_records"`
	Documents         int      `json:"documents"`
	SkippedDuplicates int      `json:"skipped_duplicates"`
	Failed            int      `json:"failed"`
	Errors            []string `json:"errors"`
}

func (res *importResult) fail(format string, args ...any) {
	res.Failed++
	if len(res.Errors) < 20 {
		res.Errors = append(res.Errors, fmt.Sprintf(format, args...))
	}
}

// clip shortens s to n runes (never splitting a UTF-8 sequence).
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func productKey(p domain.Product) string {
	return strings.ToLower(p.Name) + "|" + p.PurchaseDate.String() + "|" + strings.ToLower(p.SerialNumber)
}

func fieldList(errs map[string]string) string {
	keys := make([]string, 0, len(errs))
	for k := range errs {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

// handleImport restores a backup archive (request body = the ZIP) into the current account.
// Products already present (same name, purchase date and serial number) are skipped, so
// importing the same archive twice does not create duplicates.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	limit := s.MaxImport
	if limit <= 0 {
		limit = DefaultMaxImport
	}
	tmp, err := os.CreateTemp("", "ww-import-*.zip")
	if err != nil {
		s.storeError(w, err)
		return
	}
	defer func() { tmp.Close(); os.Remove(tmp.Name()) }()
	n, err := io.Copy(tmp, http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("backup exceeds %d MB", limit>>20), nil)
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", "could not read the upload", nil)
		return
	}
	invalid := func(msg string) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_backup", msg, nil)
	}
	zr, err := zip.NewReader(tmp, n)
	if err != nil {
		invalid("not a Warranty Wallet backup (zip expected)")
		return
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	var bf backupFile
	mf := files[backupManifest]
	if mf == nil || mf.UncompressedSize64 > maxManifestBytes {
		invalid("backup.json missing")
		return
	}
	if err := readZipJSON(mf, &bf); err != nil || bf.Format != backupFormat {
		invalid("backup.json is not a Warranty Wallet backup")
		return
	}
	if bf.Version != backupVersion {
		invalid(fmt.Sprintf("unsupported backup version %d", bf.Version))
		return
	}
	if len(bf.Products) > maxBackupProducts {
		invalid(fmt.Sprintf("at most %d products per backup", maxBackupProducts))
		return
	}

	existing, err := s.Store.ListProducts(ctx, u.ID, storeFilterAll)
	if err != nil {
		s.storeError(w, err)
		return
	}
	seen := map[string]bool{}
	for _, p := range existing {
		seen[productKey(p)] = true
	}
	res := importResult{Errors: []string{}}
	today := s.today()
	for _, bp := range bf.Products {
		p := domain.Product{UserID: u.ID}
		bp.productInput.apply(&p)
		if seen[productKey(p)] {
			res.SkippedDuplicates++
			continue
		}
		if errs := s.validateProduct(ctx, &p); len(errs) > 0 {
			res.fail("%q atlandı: geçersiz alanlar (%s)", clip(p.Name, 80), fieldList(errs))
			continue
		}
		if err := s.Store.CreateProduct(ctx, &p); err != nil {
			s.storeError(w, err)
			return
		}
		seen[productKey(p)] = true
		s.syncReminders(ctx, p)
		res.Products++

		recIDs := map[string]string{}
		for _, bs := range bp.ServiceRecords {
			rec := domain.ServiceRecord{UserID: u.ID, ProductID: p.ID, UnderWarranty: bs.UnderWarranty}
			in := serviceInput{Problem: &bs.Problem, Description: &bs.Description, ServiceName: &bs.ServiceName,
				ServiceDate: &bs.ServiceDate, Status: &bs.Status, Notes: &bs.Notes, Cost: &bs.Cost}
			if errs := in.apply(&rec, today); len(errs) > 0 {
				res.fail("%q servis kaydı atlandı: geçersiz alanlar (%s)", clip(p.Name, 80), fieldList(errs))
				continue
			}
			if rec.Status.Closed() {
				rec.ResolvedAt = bs.ResolvedAt
				if rec.ResolvedAt == nil {
					s.markResolution(&rec)
				}
			}
			if err := s.Store.CreateServiceRecord(ctx, &rec); err != nil {
				s.storeError(w, err)
				return
			}
			recIDs[bs.Ref] = rec.ID
			res.ServiceRecords++
			for _, be := range bs.Events {
				note := strings.TrimSpace(be.Note)
				if !domain.ValidServiceStatus(be.Status) || utf8.RuneCountInString(note) > 500 {
					continue
				}
				e := &domain.ServiceEvent{ServiceRecordID: rec.ID, Status: be.Status, Note: note, CreatedAt: be.CreatedAt}
				if err := s.Store.AddServiceEvent(ctx, u.ID, e); err != nil {
					s.Logger.Warn("import: service event", "err", err)
				}
			}
		}

		for _, bd := range bp.Documents {
			if err := s.importDocument(r, u.ID, p.ID, recIDs, bd, files); err != nil {
				res.fail("%q belgesi atlandı: %s", clip(bd.OriginalName, 80), err)
				continue
			}
			res.Documents++
		}
	}
	s.Logger.Info("backup imported", "products", res.Products, "documents", res.Documents, "skipped", res.SkippedDuplicates, "failed", res.Failed)
	writeJSON(w, http.StatusOK, res)
}

func readZipJSON(f *zip.File, v any) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return json.NewDecoder(io.LimitReader(rc, maxManifestBytes)).Decode(v)
}

// importDocument copies one archived file into storage. Errors are Turkish, user-facing reasons.
func (s *Server) importDocument(r *http.Request, userID, productID string, recIDs map[string]string, bd backupDocument, files map[string]*zip.File) error {
	f := files[bd.File]
	if f == nil {
		return errors.New("dosya yedekte yok")
	}
	if !domain.ValidDocumentType(bd.Type) {
		return errors.New("belge türü geçersiz")
	}
	if f.UncompressedSize64 > uint64(s.MaxUpload) {
		return errors.New("dosya çok büyük")
	}
	rc, err := f.Open()
	if err != nil {
		return errors.New("dosya okunamadı")
	}
	data, err := io.ReadAll(io.LimitReader(rc, s.MaxUpload+1))
	rc.Close()
	if err != nil || int64(len(data)) > s.MaxUpload || len(data) == 0 {
		return errors.New("dosya okunamadı")
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	ct := sniffContentType(head)
	if _, ok := allowedTypes[ct]; !ok {
		return errors.New("dosya türü desteklenmiyor")
	}
	name := strings.TrimSpace(bd.OriginalName)
	if utf8.RuneCountInString(name) > 255 {
		name = string([]rune(name)[:255])
	}
	doc := domain.Document{ID: domain.NewID(), UserID: userID, ProductID: productID, ServiceRecordID: recIDs[bd.ServiceRecordRef],
		Type: bd.Type, ContentType: ct, SizeBytes: int64(len(data)), OriginalName: name}
	doc.FileKey = fmt.Sprintf("%s/%s/%s%s", userID, productID, doc.ID, allowedTypes[ct])
	if _, err := s.Files.Put(r.Context(), doc.FileKey, bytes.NewReader(data)); err != nil {
		s.Logger.Error("import: store file", "err", err)
		return errors.New("dosya kaydedilemedi")
	}
	if err := s.Store.CreateDocument(r.Context(), &doc); err != nil {
		_ = s.Files.Delete(r.Context(), doc.FileKey)
		s.Logger.Error("import: document", "err", err)
		return errors.New("belge kaydedilemedi")
	}
	return nil
}
