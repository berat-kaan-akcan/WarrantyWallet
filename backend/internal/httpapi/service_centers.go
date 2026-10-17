package httpapi

import (
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

var phoneRe = regexp.MustCompile(`^[0-9+() -]{0,30}$`)

type centerView struct {
	domain.ServiceCenter
	DistanceKM *float64 `json:"distance_km,omitempty"`
}

// mapsSearchURL is the fallback when no center is on file: a Google Maps search that needs no API key.
func mapsSearchURL(brand, city string) string {
	q := strings.TrimSpace(strings.TrimSpace(brand) + " yetkili servis " + strings.TrimSpace(city))
	return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(q)
}

// haversineKM is the great-circle distance between two coordinates.
func haversineKM(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*rad, (lng2-lng1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

// parsePoint reads optional lat/lng query parameters.
func parsePoint(q url.Values) (lat, lng float64, ok bool, valid bool) {
	ls, gs := q.Get("lat"), q.Get("lng")
	if ls == "" && gs == "" {
		return 0, 0, false, true
	}
	lat, err1 := strconv.ParseFloat(ls, 64)
	lng, err2 := strconv.ParseFloat(gs, 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return 0, 0, false, false
	}
	return lat, lng, true, true
}

// findCenters lists centers for brand (and city). cityOnly allows a city-wide list without a brand.
func (s *Server) findCenters(w http.ResponseWriter, r *http.Request, brand string, cityOnly bool, extra map[string]any) {
	q := r.URL.Query()
	city := strings.TrimSpace(q.Get("city"))
	if utf8.RuneCountInString(city) > 60 || utf8.RuneCountInString(brand) > 60 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid filter", map[string]string{"city": "max 60 characters"})
		return
	}
	lat, lng, near, valid := parsePoint(q)
	if !valid {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid location", map[string]string{"lat": "lat and lng must be valid coordinates"})
		return
	}
	items := []centerView{}
	if strings.TrimSpace(brand) != "" || (cityOnly && city != "") {
		centers, err := s.Store.ListServiceCenters(r.Context(), brand, city)
		if err != nil {
			s.storeError(w, err)
			return
		}
		for _, c := range centers {
			v := centerView{ServiceCenter: c}
			if near && c.Lat != nil && c.Lng != nil {
				d := math.Round(haversineKM(lat, lng, *c.Lat, *c.Lng)*10) / 10
				v.DistanceKM = &d
			}
			items = append(items, v)
		}
		if near { // nearest first; centers without coordinates last
			sort.SliceStable(items, func(i, j int) bool {
				a, b := items[i].DistanceKM, items[j].DistanceKM
				if (a == nil) != (b == nil) {
					return b == nil
				}
				return a != nil && *a < *b
			})
		}
	}
	resp := map[string]any{"items": items, "maps_search_url": mapsSearchURL(brand, city)}
	for k, v := range extra {
		resp[k] = v
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleServiceCenters: GET /service-centers?brand=&city=&lat=&lng=
func (s *Server) handleServiceCenters(w http.ResponseWriter, r *http.Request) {
	s.findCenters(w, r, strings.TrimSpace(r.URL.Query().Get("brand")), true, nil)
}

// handleProductServiceCenters finds centers for the product's brand.
func (s *Server) handleProductServiceCenters(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.GetProduct(r.Context(), userFrom(r.Context()).ID, r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	brand := p.Brand
	extra := map[string]any{"brand": brand}
	if brand == "" { // no brand: the map search uses the product name instead
		extra["maps_search_url"] = mapsSearchURL(p.Name, r.URL.Query().Get("city"))
	}
	s.findCenters(w, r, brand, false, extra)
}

func (s *Server) readCenter(w http.ResponseWriter, r *http.Request) (domain.ServiceCenter, bool) {
	var c domain.ServiceCenter
	if !decodeJSON(w, r, &c) {
		return c, false
	}
	for _, f := range []*string{&c.Brand, &c.Name, &c.City, &c.District, &c.Address, &c.Phone, &c.Website} {
		*f = strings.TrimSpace(*f)
	}
	errs := map[string]string{}
	need := func(field, v string, max int) {
		if v == "" || utf8.RuneCountInString(v) > max {
			errs[field] = "required, max " + strconv.Itoa(max) + " characters"
		}
	}
	need("brand", c.Brand, 60)
	need("name", c.Name, 120)
	need("city", c.City, 60)
	if utf8.RuneCountInString(c.District) > 60 {
		errs["district"] = "max 60 characters"
	}
	if utf8.RuneCountInString(c.Address) > 300 {
		errs["address"] = "max 300 characters"
	}
	if !phoneRe.MatchString(c.Phone) {
		errs["phone"] = "digits, spaces, + ( ) - only"
	}
	if c.Website != "" {
		u, err := url.Parse(c.Website)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(c.Website) > 200 {
			errs["website"] = "http(s) URL, max 200 characters"
		}
	}
	if (c.Lat == nil) != (c.Lng == nil) || (c.Lat != nil && (*c.Lat < -90 || *c.Lat > 90 || *c.Lng < -180 || *c.Lng > 180)) {
		errs["lat"] = "lat and lng together, valid coordinates"
	}
	if len(errs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid service center", errs)
		return c, false
	}
	return c, true
}

func (s *Server) handleAdminServiceCenters(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := s.Store.ListServiceCenters(r.Context(), q.Get("brand"), q.Get("city"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAdminCreateServiceCenter(w http.ResponseWriter, r *http.Request) {
	c, ok := s.readCenter(w, r)
	if !ok {
		return
	}
	c.ID = ""
	if err := s.Store.CreateServiceCenter(r.Context(), &c); err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleAdminUpdateServiceCenter(w http.ResponseWriter, r *http.Request) {
	c, ok := s.readCenter(w, r)
	if !ok {
		return
	}
	c.ID = r.PathValue("id")
	if err := s.Store.UpdateServiceCenter(r.Context(), &c); err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleAdminDeleteServiceCenter(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteServiceCenter(r.Context(), r.PathValue("id")); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
