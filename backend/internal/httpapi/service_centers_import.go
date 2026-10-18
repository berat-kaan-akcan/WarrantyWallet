package httpapi

import (
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

// MaxCenterImportRows caps one CSV import.
const MaxCenterImportRows = 5000

var centerColumns = map[string]string{
	"brand": "brand", "marka": "brand",
	"name": "name", "ad": "name", "servis adi": "name",
	"city": "city", "sehir": "city",
	"district": "district", "ilce": "district",
	"address": "address", "adres": "address",
	"phone": "phone", "telefon": "phone",
	"website": "website", "web sitesi": "website", "web": "website",
	"lat": "lat", "enlem": "lat",
	"lng": "lng", "boylam": "lng",
}

// parseCentersCSV reads a header row (English or Turkish column names, any order) and the rows below it.
// Excel in Turkish locale writes ";" separated files with decimal commas, so both are accepted.
// Errors are keyed "row N" (N = line in the file, header = 1).
func parseCentersCSV(text string) ([]domain.ServiceCenter, map[string]string) {
	text = strings.TrimPrefix(text, "\ufeff")
	first, _, _ := strings.Cut(text, "\n")
	sep := ','
	if strings.Count(first, ";") > strings.Count(first, ",") {
		sep = ';'
	}
	rd := csv.NewReader(strings.NewReader(text))
	rd.Comma = sep
	rd.FieldsPerRecord = -1
	rd.TrimLeadingSpace = true
	header, err := rd.Read()
	if err != nil {
		return nil, map[string]string{"csv": "empty file or no header row"}
	}
	cols := map[string]int{}
	for i, h := range header {
		if f, ok := centerColumns[domain.FoldKey(h)]; ok {
			cols[f] = i
		}
	}
	for _, f := range []string{"brand", "name", "city"} {
		if _, ok := cols[f]; !ok {
			return nil, map[string]string{"csv": "header needs brand, name and city columns (marka, ad, sehir)"}
		}
	}
	errs := map[string]string{}
	var out []domain.ServiceCenter
	for line := 2; ; line++ {
		rec, err := rd.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		key := "row " + strconv.Itoa(line)
		if err != nil {
			errs[key] = "unreadable CSV line"
			break
		}
		get := func(f string) string {
			if i, ok := cols[f]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		if strings.Join(rec, "") == "" {
			continue // blank line
		}
		if len(out) == MaxCenterImportRows {
			errs["csv"] = "at most " + strconv.Itoa(MaxCenterImportRows) + " rows per import"
			break
		}
		c := domain.ServiceCenter{Brand: get("brand"), Name: get("name"), City: get("city"), District: get("district"),
			Address: get("address"), Phone: get("phone"), Website: get("website")}
		bad := false
		coord := func(f string) *float64 {
			v := get(f)
			if v == "" {
				return nil
			}
			n, err := strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
			if err != nil {
				bad = true
				return nil
			}
			return &n
		}
		c.Lat, c.Lng = coord("lat"), coord("lng")
		fe := cleanCenter(&c)
		if bad {
			fe["lat"] = "lat and lng must be numbers"
		}
		if len(fe) > 0 {
			names := make([]string, 0, len(fe))
			for k := range fe {
				names = append(names, k)
			}
			sort.Strings(names)
			errs[key] = "invalid " + strings.Join(names, ", ")
			if len(errs) >= 20 { // enough to fix the file; don't flood the response
				break
			}
			continue
		}
		out = append(out, c)
	}
	return out, errs
}

// handleAdminImportServiceCenters: POST /admin/service-centers/import {"csv": "..."}.
// All rows are validated first; nothing is written if any row is invalid. Rows matching an
// existing center (same brand, name and city, ignoring case and Turkish letters) are skipped,
// so the same file can be imported again safely.
func (s *Server) handleAdminImportServiceCenters(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CSV string `json:"csv"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	rows, errs := parseCentersCSV(body.CSV)
	if len(errs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid CSV", errs)
		return
	}
	existing, err := s.Store.ListServiceCenters(r.Context(), "", "")
	if err != nil {
		s.storeError(w, err)
		return
	}
	seen := map[string]bool{}
	ident := func(c domain.ServiceCenter) string {
		return domain.FoldKey(c.Brand) + "\x00" + domain.FoldKey(c.Name) + "\x00" + domain.FoldKey(c.City)
	}
	for _, c := range existing {
		seen[ident(c)] = true
	}
	created, skipped := 0, 0
	for _, c := range rows {
		if seen[ident(c)] {
			skipped++
			continue
		}
		if err := s.Store.CreateServiceCenter(r.Context(), &c); err != nil {
			s.storeError(w, err)
			return
		}
		seen[ident(c)] = true
		created++
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": created, "skipped": skipped})
}
