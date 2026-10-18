package httpapi

import (
	"math"
	"net/url"
	"strings"
	"testing"
)

func TestHaversine(t *testing.T) {
	// Kadıköy → Beşiktaş is roughly 5.5 km
	if d := haversineKM(40.9909, 29.0303, 41.0422, 29.0083); math.Abs(d-6.0) > 1 {
		t.Fatalf("distance %.2f", d)
	}
	u := mapsSearchURL("ASUS", "İstanbul")
	parsed, _ := url.Parse(u)
	if parsed.Host != "www.google.com" || parsed.Query().Get("query") != "ASUS yetkili servis İstanbul" {
		t.Fatalf("maps url %s", u)
	}
}

func TestServiceCenterFinder(t *testing.T) {
	h := newHarness(t)
	adm := h.register("admin@ww.test", "Admin")
	user := h.register("user@ww.test", "U")
	center := map[string]any{"brand": "ASUS", "name": "ASUS Kadıköy", "city": "İstanbul", "district": "Kadıköy",
		"phone": "+90 216 000 00 00", "website": "https://example.com/servis", "lat": 40.9909, "lng": 29.0303}
	if r := h.do("POST", "/api/v1/admin/service-centers", user, center); r.Status != 403 {
		t.Fatalf("non-admin create %d", r.Status)
	}
	if r := h.do("POST", "/api/v1/admin/service-centers", adm, map[string]any{"brand": "", "name": "x", "city": "y",
		"phone": "call me", "website": "javascript:alert(1)", "lat": 41}); r.Status != 422 {
		t.Fatalf("validation %d", r.Status)
	} else {
		f := r.json(t)["error"].(map[string]any)["fields"].(map[string]any)
		for _, k := range []string{"brand", "phone", "website", "lat"} {
			if f[k] == nil {
				t.Fatalf("missing field error %s: %v", k, f)
			}
		}
	}
	r := h.do("POST", "/api/v1/admin/service-centers", adm, center)
	if r.Status != 201 {
		t.Fatalf("create %d %s", r.Status, r.Body)
	}
	id := r.json(t)["id"].(string)
	h.do("POST", "/api/v1/admin/service-centers", adm, map[string]any{"brand": "Asus", "name": "ASUS Beşiktaş", "city": "istanbul",
		"lat": 41.0422, "lng": 29.0083})
	h.do("POST", "/api/v1/admin/service-centers", adm, map[string]any{"brand": "ASUS", "name": "ASUS Mecidiyeköy", "city": "ISTANBUL"})
	h.do("POST", "/api/v1/admin/service-centers", adm, map[string]any{"brand": "ASUS", "name": "ASUS Ankara", "city": "Ankara"})

	pid := h.do("POST", "/api/v1/products", user, monitor).json(t)["id"].(string)
	// near Beşiktaş: Beşiktaş first, Kadıköy second, the one without coordinates last
	res := h.do("GET", "/api/v1/products/"+pid+"/service-centers?city=%C4%B0stanbul&lat=41.045&lng=29.0", user, nil).json(t)
	items := res["items"].([]any)
	if res["brand"] != "ASUS" || len(items) != 3 {
		t.Fatalf("result %v", res)
	}
	names := []string{}
	for _, it := range items {
		names = append(names, it.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "ASUS Beşiktaş,ASUS Kadıköy,ASUS Mecidiyeköy" {
		t.Fatalf("order %v", names)
	}
	if items[0].(map[string]any)["distance_km"] == nil || items[2].(map[string]any)["distance_km"] != nil {
		t.Fatalf("distances %v", items)
	}
	if !strings.Contains(res["maps_search_url"].(string), "ASUS+yetkili+servis") {
		t.Fatalf("maps url %v", res["maps_search_url"])
	}
	// without city: all ASUS centers, by city then name
	if all := h.do("GET", "/api/v1/products/"+pid+"/service-centers", user, nil).json(t)["items"].([]any); len(all) != 4 {
		t.Fatalf("all %d", len(all))
	}
	if r := h.do("GET", "/api/v1/service-centers?brand=asus&city=ankara", user, nil); len(r.json(t)["items"].([]any)) != 1 {
		t.Fatalf("generic search %s", r.Body)
	}
	if r := h.do("GET", "/api/v1/service-centers", user, nil); len(r.json(t)["items"].([]any)) != 0 {
		t.Fatalf("no filter should not dump the table %s", r.Body)
	}
	if r := h.do("GET", "/api/v1/service-centers?brand=asus&lat=100&lng=0", user, nil); r.Status != 422 {
		t.Fatalf("bad lat %d", r.Status)
	}
	// update and delete
	center["name"] = "ASUS Kadıköy Yeni"
	if r := h.do("PUT", "/api/v1/admin/service-centers/"+id, adm, center); r.Status != 200 || r.json(t)["name"] != "ASUS Kadıköy Yeni" {
		t.Fatalf("update %d %s", r.Status, r.Body)
	}
	if r := h.do("DELETE", "/api/v1/admin/service-centers/"+id, adm, nil); r.Status != 204 {
		t.Fatalf("delete %d", r.Status)
	}
	if r := h.do("GET", "/api/v1/admin/service-centers?brand=asus", adm, nil); len(r.json(t)["items"].([]any)) != 3 {
		t.Fatalf("admin list %s", r.Body)
	}
	// product without brand: map search uses the product name
	noBrand := h.do("POST", "/api/v1/products", user, map[string]any{"name": "Kahve makinesi", "category": "other",
		"purchase_date": "2026-01-01", "currency": "TRY"}).json(t)["id"].(string)
	nb := h.do("GET", "/api/v1/products/"+noBrand+"/service-centers?city=Ankara", user, nil).json(t)
	if len(nb["items"].([]any)) != 0 || !strings.Contains(nb["maps_search_url"].(string), "Kahve+makinesi+yetkili+servis+Ankara") {
		t.Fatalf("no brand %v", nb)
	}
}

func TestServiceCenterImport(t *testing.T) {
	h := newHarness(t)
	adm := h.register("admin@ww.test", "Admin")
	user := h.register("user@ww.test", "U")
	path := "/api/v1/admin/service-centers/import"
	good := "\ufeffMarka;Ad;Şehir;İlçe;Telefon;Enlem;Boylam\n" +
		"ASUS;ASUS Kadıköy;İstanbul;Kadıköy;+90 216 000 00 00;40,9909;29,0303\n" +
		"\n" +
		"Arçelik;Arçelik Çankaya;Ankara;;;;\n"
	if r := h.do("POST", path, user, map[string]any{"csv": good}); r.Status != 403 {
		t.Fatalf("non-admin import %d", r.Status)
	}
	bad := "brand,name,city,phone,lat,lng\nASUS,A,İstanbul,call me,,\nASUS,B,İstanbul,,41,\n,C,Ankara,,,\nASUS,D,İzmir,,x,1\n"
	r := h.do("POST", path, adm, map[string]any{"csv": bad})
	if r.Status != 422 {
		t.Fatalf("bad csv %d %s", r.Status, r.Body)
	}
	f := r.json(t)["error"].(map[string]any)["fields"].(map[string]any)
	if len(f) != 4 || !strings.Contains(f["row 2"].(string), "phone") || !strings.Contains(f["row 3"].(string), "lat") ||
		!strings.Contains(f["row 4"].(string), "brand") || !strings.Contains(f["row 5"].(string), "lat") {
		t.Fatalf("row errors %v", f)
	}
	if r := h.do("POST", path, adm, map[string]any{"csv": "x,y\n1,2\n"}); r.Status != 422 {
		t.Fatalf("missing columns %d", r.Status)
	}
	if n := len(h.do("GET", "/api/v1/admin/service-centers", adm, nil).json(t)["items"].([]any)); n != 0 {
		t.Fatalf("invalid file must write nothing, got %d", n)
	}
	res := h.do("POST", path, adm, map[string]any{"csv": good}).json(t)
	if res["created"] != 2.0 || res["skipped"] != 0.0 {
		t.Fatalf("import %v", res)
	}
	// importing again (different case) skips the existing rows
	again := strings.Replace(good, "ASUS Kadıköy;İstanbul", "asus kadıköy;ISTANBUL", 1)
	if res := h.do("POST", path, adm, map[string]any{"csv": again}).json(t); res["created"] != 0.0 || res["skipped"] != 2.0 {
		t.Fatalf("re-import %v", res)
	}
	items := h.do("GET", "/api/v1/service-centers?brand=asus&city=istanbul", user, nil).json(t)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["lat"] != 40.9909 || items[0].(map[string]any)["district"] != "Kadıköy" {
		t.Fatalf("imported center %v", items)
	}
}
