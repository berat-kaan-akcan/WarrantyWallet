package httpapi

import (
	"strings"
	"testing"
	"time"
)

func TestInviteCodes(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := newInviteCode()
		if err != nil || len(c) != 9 || c[4] != '-' || strings.ContainsAny(c, "01ILO") {
			t.Fatalf("code %q %v", c, err)
		}
		seen[c] = true
	}
	if len(seen) < 199 {
		t.Fatal("codes repeat")
	}
	if normalizeInviteCode(" k7qm-2xrp ") != "K7QM2XRP" || inviteHash("k7qm 2xrp") != inviteHash("K7QM-2XRP") {
		t.Fatal("normalization")
	}
}

func TestFamilyFlow(t *testing.T) {
	h := newHarness(t)
	h.api.authLimiter.limit = 100
	owner := h.register("anne@ww.test", "Ayşe Akcan")
	kid := h.register("cocuk@ww.test", "Kaan")
	stranger := h.register("baska@ww.test", "Başka")
	h.do("POST", "/api/v1/products", owner, monitor)
	h.do("POST", "/api/v1/products", kid, map[string]any{"name": "iPhone 16", "brand": "Apple", "category": "phone",
		"purchase_date": "2025-11-01", "price": 70000, "currency": "TRY", "warranty_months": 12})
	h.do("POST", "/api/v1/products", stranger, map[string]any{"name": "Gizli", "category": "other", "purchase_date": "2026-01-01", "currency": "TRY"})

	if r := h.do("GET", "/api/v1/family", owner, nil); r.Status != 200 || r.json(t)["family"] != nil {
		t.Fatalf("no family %d %s", r.Status, r.Body)
	}
	if r := h.do("GET", "/api/v1/family/products", owner, nil); r.Status != 404 {
		t.Fatalf("products without family %d", r.Status)
	}
	if r := h.do("POST", "/api/v1/family", owner, map[string]string{"name": " "}); r.Status != 422 {
		t.Fatalf("empty name %d", r.Status)
	}
	r := h.do("POST", "/api/v1/family", owner, map[string]string{"name": "Akcan ailesi"})
	if r.Status != 201 || r.json(t)["role"] != "owner" {
		t.Fatalf("create %d %s", r.Status, r.Body)
	}
	if r := h.do("POST", "/api/v1/family", owner, map[string]string{"name": "x"}); r.Status != 409 {
		t.Fatalf("second family %d", r.Status)
	}
	inv := h.do("POST", "/api/v1/family/invites", owner, nil)
	if inv.Status != 201 {
		t.Fatalf("invite %d %s", inv.Status, inv.Body)
	}
	code := inv.json(t)["code"].(string)
	if r := h.do("POST", "/api/v1/family/join", kid, map[string]string{"code": "ABCD-EFGH"}); r.Status != 400 {
		t.Fatalf("wrong code %d", r.Status)
	}
	j := h.do("POST", "/api/v1/family/join", kid, map[string]string{"code": strings.ToLower(code)})
	if j.Status != 200 || j.json(t)["role"] != "member" || len(j.json(t)["members"].([]any)) != 2 {
		t.Fatalf("join %d %s", j.Status, j.Body)
	}
	if r := h.do("POST", "/api/v1/family/join", stranger, map[string]string{"code": code}); r.Status != 400 {
		t.Fatalf("code reused %d", r.Status)
	}
	// members cannot invite, rename or remove others
	if r := h.do("POST", "/api/v1/family/invites", kid, nil); r.Status != 403 {
		t.Fatalf("member invite %d", r.Status)
	}
	if r := h.do("PATCH", "/api/v1/family", kid, map[string]string{"name": "x"}); r.Status != 403 {
		t.Fatalf("member rename %d", r.Status)
	}
	members := j.json(t)["members"].([]any)
	ownerID := members[0].(map[string]any)["id"].(string)
	kidID := members[1].(map[string]any)["id"].(string)
	if r := h.do("DELETE", "/api/v1/family/members/"+ownerID, kid, nil); r.Status != 403 {
		t.Fatalf("member removes owner %d", r.Status)
	}

	fp := h.do("GET", "/api/v1/family/products", kid, nil).json(t)["items"].([]any)
	if len(fp) != 2 {
		t.Fatalf("family products %v", fp)
	}
	first := fp[0].(map[string]any) // iPhone ends 2026-11-01, sooner than the monitor
	if first["name"] != "iPhone 16" || first["owner"].(map[string]any)["name"] != "Kaan" || first["status"] != "expiring_soon" {
		t.Fatalf("order/owner %v", first)
	}
	if strings.Contains(string(h.do("GET", "/api/v1/family/products", owner, nil).Body), "Gizli") {
		t.Fatal("stranger's product visible")
	}
	if r := h.do("GET", "/api/v1/family/products?status=expired", owner, nil); len(r.json(t)["items"].([]any)) != 0 {
		t.Fatalf("status filter %s", r.Body)
	}
	if r := h.do("GET", "/api/v1/family/products?status=bogus", owner, nil); r.Status != 422 {
		t.Fatalf("bad filter %d", r.Status)
	}
	// family members still cannot open each other's products directly
	kidProduct := first["id"].(string)
	if r := h.do("GET", "/api/v1/products/"+kidProduct, owner, nil); r.Status != 404 {
		t.Fatalf("direct access %d", r.Status)
	}

	if r := h.do("PATCH", "/api/v1/family", owner, map[string]string{"name": "Ev"}); r.Status != 200 || r.json(t)["family"].(map[string]any)["name"] != "Ev" {
		t.Fatalf("rename %d %s", r.Status, r.Body)
	}
	if r := h.do("DELETE", "/api/v1/family/members/"+kidID, owner, nil); r.Status != 204 {
		t.Fatalf("owner removes kid %d", r.Status)
	}
	if r := h.do("GET", "/api/v1/family", kid, nil); r.json(t)["family"] != nil {
		t.Fatal("kid still in family")
	}

	// expired invite
	inv2 := h.do("POST", "/api/v1/family/invites", owner, nil).json(t)["code"].(string)
	h.api.Now = func() time.Time { return testNow.Add(8 * 24 * time.Hour) }
	if r := h.do("POST", "/api/v1/family/join", kid, map[string]string{"code": inv2}); r.Status != 400 {
		t.Fatalf("expired invite %d", r.Status)
	}
	h.api.Now = func() time.Time { return testNow }

	// owner leaving dissolves the family; outstanding invites die with it
	inv3 := h.do("POST", "/api/v1/family/invites", owner, nil).json(t)["code"].(string)
	if r := h.do("DELETE", "/api/v1/family/members/"+ownerID, owner, nil); r.Status != 204 {
		t.Fatalf("owner leaves %d", r.Status)
	}
	if r := h.do("POST", "/api/v1/family/join", kid, map[string]string{"code": inv3}); r.Status != 400 {
		t.Fatalf("invite of dissolved family %d", r.Status)
	}
	if r := h.do("DELETE", "/api/v1/family/members/"+ownerID, owner, nil); r.Status != 404 {
		t.Fatalf("leave without family %d", r.Status)
	}
}

func TestFamilyIsLimited(t *testing.T) {
	h := newHarness(t)
	h.api.authLimiter.limit = 1000
	owner := h.register("o@ww.test", "O")
	h.do("POST", "/api/v1/family", owner, map[string]string{"name": "Büyük aile"})
	for i := 1; i < 8; i++ {
		code := h.do("POST", "/api/v1/family/invites", owner, nil).json(t)["code"].(string)
		m := h.register("m"+string(rune('a'+i))+"@ww.test", "M")
		if r := h.do("POST", "/api/v1/family/join", m, map[string]string{"code": code}); r.Status != 200 {
			t.Fatalf("join %d: %d", i, r.Status)
		}
	}
	if r := h.do("POST", "/api/v1/family/invites", owner, nil); r.Status != 409 || !strings.Contains(string(r.Body), "family_full") {
		t.Fatalf("ninth invite %d %s", r.Status, r.Body)
	}
}
