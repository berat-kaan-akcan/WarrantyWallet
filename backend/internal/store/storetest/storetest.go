// Package storetest is a contract test suite every store.Store implementation must pass.
package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store"
)

// Run executes the contract suite. newStore must return an empty store.
func Run(t *testing.T, newStore func(t *testing.T) store.Store) {
	ctx := context.Background()

	t.Run("users", func(t *testing.T) {
		s := newStore(t)
		u := &domain.User{Email: "Ali@Example.com", Name: "Ali", PasswordHash: "h"}
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		if u.ID == "" || u.Role != domain.RoleUser {
			t.Fatalf("defaults not applied: %+v", u)
		}
		if err := s.CreateUser(ctx, &domain.User{Email: "ali@example.com", PasswordHash: "h"}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("want conflict, got %v", err)
		}
		got, err := s.GetUserByEmail(ctx, " ALI@example.com")
		if err != nil || got.ID != u.ID || got.PasswordHash != "h" {
			t.Fatalf("by email: %v %+v", err, got)
		}
		if _, err := s.GetUserByID(ctx, domain.NewID()); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("want not found, got %v", err)
		}
		if err := s.SetUserRole(ctx, u.ID, domain.RoleAdmin); err != nil {
			t.Fatal(err)
		}
		got, _ = s.GetUserByID(ctx, u.ID)
		if got.Role != domain.RoleAdmin {
			t.Fatal("role not updated")
		}
		list, _ := s.ListUsers(ctx)
		if len(list) != 1 {
			t.Fatalf("list users %d", len(list))
		}
	})

	t.Run("service_centers", func(t *testing.T) {
		s := newStore(t)
		lat, lng := 41.0, 29.0
		a := &domain.ServiceCenter{Brand: "ASUS", Name: "Merkez Servis", City: "İstanbul", District: "Kadıköy", Lat: &lat, Lng: &lng}
		if err := s.CreateServiceCenter(ctx, a); err != nil || a.ID == "" || a.CreatedAt.IsZero() {
			t.Fatalf("create %v %+v", err, a)
		}
		_ = s.CreateServiceCenter(ctx, &domain.ServiceCenter{Brand: "Asus", Name: "Ankara Servis", City: "Ankara"})
		_ = s.CreateServiceCenter(ctx, &domain.ServiceCenter{Brand: "Arçelik", Name: "Arçelik Servis", City: "ISTANBUL"})
		if l, _ := s.ListServiceCenters(ctx, "asus", ""); len(l) != 2 || l[0].City != "Ankara" {
			t.Fatalf("by brand %+v", l)
		}
		if l, _ := s.ListServiceCenters(ctx, "", "istanbul"); len(l) != 2 {
			t.Fatalf("by city (folded) %+v", l)
		}
		l, _ := s.ListServiceCenters(ctx, "ARCELIK", "İstanbul")
		if len(l) != 1 || l[0].Lat != nil {
			t.Fatalf("brand+city %+v", l)
		}
		got, err := s.GetServiceCenter(ctx, a.ID)
		if err != nil || got.Lat == nil || *got.Lat != 41.0 || got.District != "Kadıköy" {
			t.Fatalf("get %+v %v", got, err)
		}
		got.Name, got.City, got.Lat, got.Lng = "Yeni Ad", "İzmir", nil, nil
		if err := s.UpdateServiceCenter(ctx, &got); err != nil {
			t.Fatal(err)
		}
		if l, _ := s.ListServiceCenters(ctx, "asus", "izmir"); len(l) != 1 || l[0].Name != "Yeni Ad" || l[0].Lat != nil {
			t.Fatalf("after update %+v", l)
		}
		if err := s.UpdateServiceCenter(ctx, &domain.ServiceCenter{ID: domain.NewID()}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing %v", err)
		}
		if err := s.DeleteServiceCenter(ctx, a.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetServiceCenter(ctx, a.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("not deleted")
		}
		if err := s.DeleteServiceCenter(ctx, a.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("delete twice %v", err)
		}
	})

	t.Run("families", func(t *testing.T) {
		s := newStore(t)
		a, b, c := mkUser(t, s, "fam-a@x.io"), mkUser(t, s, "fam-b@x.io"), mkUser(t, s, "fam-c@x.io")
		if _, err := s.GetFamilyForUser(ctx, a); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("no family yet %v", err)
		}
		f := &domain.Family{Name: "Akcan ailesi", OwnerID: a}
		if err := s.CreateFamily(ctx, f); err != nil || f.ID == "" || f.CreatedAt.IsZero() {
			t.Fatalf("create %v %+v", err, f)
		}
		if err := s.CreateFamily(ctx, &domain.Family{Name: "İkinci", OwnerID: a}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("second family %v", err)
		}
		if err := s.AddFamilyMember(ctx, f.ID, b); err != nil {
			t.Fatal(err)
		}
		if err := s.AddFamilyMember(ctx, f.ID, b); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("double join %v", err)
		}
		if err := s.AddFamilyMember(ctx, domain.NewID(), c); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("unknown family %v", err)
		}
		if err := s.CreateFamily(ctx, &domain.Family{Name: "B", OwnerID: b}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("member creating a family %v", err)
		}
		got, err := s.GetFamilyForUser(ctx, b)
		if err != nil || got.ID != f.ID || got.OwnerID != a || got.Name != "Akcan ailesi" {
			t.Fatalf("family of b %+v %v", got, err)
		}
		_ = s.RenameFamily(ctx, f.ID, "Ev")
		ms, _ := s.ListFamilyMembers(ctx, f.ID)
		if len(ms) != 2 || ms[0].UserID != a || ms[0].Role != domain.FamilyOwner || ms[1].Email != "fam-b@x.io" || ms[1].Role != domain.FamilyMember {
			t.Fatalf("members %+v", ms)
		}
		if g, _ := s.GetFamilyForUser(ctx, a); g.Name != "Ev" {
			t.Fatalf("rename %+v", g)
		}
		if err := s.RemoveFamilyMember(ctx, f.ID, c); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("remove non-member %v", err)
		}
		if err := s.RemoveFamilyMember(ctx, f.ID, b); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetFamilyForUser(ctx, b); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("b still in family")
		}
		_ = s.AddFamilyMember(ctx, f.ID, c)
		if err := s.DeleteUser(ctx, a); err != nil { // owner deleted → family gone
			t.Fatal(err)
		}
		if _, err := s.GetFamilyForUser(ctx, c); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("family survived its owner")
		}
		f2 := &domain.Family{Name: "C", OwnerID: c}
		if err := s.CreateFamily(ctx, f2); err != nil {
			t.Fatal(err)
		}
		_ = s.AddFamilyMember(ctx, f2.ID, b)
		if err := s.DeleteFamily(ctx, f2.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetFamilyForUser(ctx, b); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("membership survived family deletion")
		}
	})

	t.Run("device_tokens", func(t *testing.T) {
		s := newStore(t)
		a, b := mkUser(t, s, "dev-a@x.io"), mkUser(t, s, "dev-b@x.io")
		if err := s.SaveDeviceToken(ctx, &domain.DeviceToken{UserID: a, Token: "tok-1", Platform: "android"}); err != nil {
			t.Fatal(err)
		}
		d := &domain.DeviceToken{UserID: a, Token: "tok-2", Platform: "ios"}
		if err := s.SaveDeviceToken(ctx, d); err != nil || d.CreatedAt.IsZero() {
			t.Fatalf("save %v %+v", err, d)
		}
		if list, _ := s.ListDeviceTokens(ctx, a); len(list) != 2 {
			t.Fatalf("list %+v", list)
		}
		// same install signs in as b → token moves
		if err := s.SaveDeviceToken(ctx, &domain.DeviceToken{UserID: b, Token: "tok-1", Platform: "android"}); err != nil {
			t.Fatal(err)
		}
		if list, _ := s.ListDeviceTokens(ctx, a); len(list) != 1 || list[0].Token != "tok-2" || list[0].Platform != "ios" {
			t.Fatalf("after move a %+v", list)
		}
		if err := s.DeleteDeviceToken(ctx, a, "tok-1"); err != nil { // not a's token any more
			t.Fatal(err)
		}
		if list, _ := s.ListDeviceTokens(ctx, b); len(list) != 1 {
			t.Fatalf("other user's token deleted %+v", list)
		}
		_ = s.DeleteDeviceToken(ctx, "", "tok-1")
		_ = s.DeleteDeviceToken(ctx, a, "tok-2")
		if l1, _ := s.ListDeviceTokens(ctx, a); len(l1) != 0 {
			t.Fatalf("a %+v", l1)
		}
		if l2, _ := s.ListDeviceTokens(ctx, b); len(l2) != 0 {
			t.Fatalf("b %+v", l2)
		}
		if err := s.SaveDeviceToken(ctx, &domain.DeviceToken{UserID: domain.NewID(), Token: "x", Platform: "ios"}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("unknown user %v", err)
		}
		_ = s.SaveDeviceToken(ctx, &domain.DeviceToken{UserID: a, Token: "tok-3", Platform: "android"})
		if err := s.DeleteUser(ctx, a); err != nil {
			t.Fatal(err)
		}
		if l, _ := s.ListDeviceTokens(ctx, a); len(l) != 0 {
			t.Fatalf("tokens survive user deletion %+v", l)
		}
	})

	t.Run("profile_password_and_tokens", func(t *testing.T) {
		s := newStore(t)
		a := mkUser(t, s, "tok@x.io")
		if err := s.UpdateUserProfile(ctx, a, "Yeni Ad"); err != nil {
			t.Fatal(err)
		}
		if err := s.SetUserPassword(ctx, a, "newhash"); err != nil {
			t.Fatal(err)
		}
		u, _ := s.GetUserByID(ctx, a)
		if u.Name != "Yeni Ad" || u.PasswordHash != "newhash" || u.EmailVerifiedAt != nil {
			t.Fatalf("user %+v", u)
		}
		first := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
		if err := s.MarkEmailVerified(ctx, a, first); err != nil {
			t.Fatal(err)
		}
		_ = s.MarkEmailVerified(ctx, a, first.Add(time.Hour)) // keeps the first timestamp
		if u, _ := s.GetUserByID(ctx, a); u.EmailVerifiedAt == nil || !u.EmailVerifiedAt.Equal(first) {
			t.Fatalf("verified %+v", u.EmailVerifiedAt)
		}
		if err := s.MarkEmailVerified(ctx, domain.NewID(), first); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("unknown user %v", err)
		}
		vt := &domain.AuthToken{UserID: a, Kind: domain.TokenEmailVerify, TokenHash: "verify-hash", ExpiresAt: first.Add(24 * time.Hour)}
		if err := s.CreateAuthToken(ctx, vt); err != nil {
			t.Fatal(err)
		}
		if got, err := s.ConsumeAuthToken(ctx, domain.TokenEmailVerify, "verify-hash", first); err != nil || got.UserID != a {
			t.Fatalf("consume verify %v %v", got, err)
		}
		if err := s.SetUserPassword(ctx, domain.NewID(), "x"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("want not found, got %v", err)
		}
		now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
		tok := &domain.AuthToken{UserID: a, Kind: domain.TokenRefresh, TokenHash: "h1", ExpiresAt: now.Add(time.Hour)}
		if err := s.CreateAuthToken(ctx, tok); err != nil || tok.ID == "" {
			t.Fatalf("create token: %v", err)
		}
		if err := s.CreateAuthToken(ctx, &domain.AuthToken{UserID: a, Kind: domain.TokenRefresh, TokenHash: "h1", ExpiresAt: now}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate hash: %v", err)
		}
		if _, err := s.ConsumeAuthToken(ctx, domain.TokenPasswordReset, "h1", now); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("wrong kind must not consume: %v", err)
		}
		got, err := s.ConsumeAuthToken(ctx, domain.TokenRefresh, "h1", now)
		if err != nil || got.UserID != a || got.UsedAt == nil {
			t.Fatalf("consume: %v %+v", err, got)
		}
		if _, err := s.ConsumeAuthToken(ctx, domain.TokenRefresh, "h1", now); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("second use must fail: %v", err)
		}
		_ = s.CreateAuthToken(ctx, &domain.AuthToken{UserID: a, Kind: domain.TokenRefresh, TokenHash: "h2", ExpiresAt: now.Add(time.Minute)})
		if _, err := s.ConsumeAuthToken(ctx, domain.TokenRefresh, "h2", now.Add(2*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expired must fail: %v", err)
		}
		_ = s.CreateAuthToken(ctx, &domain.AuthToken{UserID: a, Kind: domain.TokenRefresh, TokenHash: "h3", ExpiresAt: now.Add(time.Hour)})
		_ = s.CreateAuthToken(ctx, &domain.AuthToken{UserID: a, Kind: domain.TokenPasswordReset, TokenHash: "h4", ExpiresAt: now.Add(time.Hour)})
		if err := s.RevokeAuthTokens(ctx, a, domain.TokenRefresh, now); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ConsumeAuthToken(ctx, domain.TokenRefresh, "h3", now); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("revoked must fail: %v", err)
		}
		if _, err := s.ConsumeAuthToken(ctx, domain.TokenPasswordReset, "h4", now); err != nil {
			t.Fatalf("other kind must survive revoke: %v", err)
		}
		if err := s.CreateAuthToken(ctx, &domain.AuthToken{UserID: domain.NewID(), Kind: domain.TokenRefresh, TokenHash: "h5", ExpiresAt: now}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("unknown user: %v", err)
		}
	})

	t.Run("product_code_lookup", func(t *testing.T) {
		s := newStore(t)
		a := mkUser(t, s, "code@x.io")
		b := mkUser(t, s, "other@x.io")
		mk := func(user, name, serial, barcode string) {
			p := &domain.Product{UserID: user, Name: name, PurchaseDate: domain.NewDate(2026, 1, 1), WarrantyMonths: 24,
				SerialNumber: serial, Barcode: barcode}
			p.Normalize()
			if err := s.CreateProduct(ctx, p); err != nil {
				t.Fatal(err)
			}
		}
		mk(a, "Sony WH-1000XM5", "5023417", "4548736132610")
		mk(a, "ASUS VG27AQ", "R9LMQS123456", "")
		mk(b, "Başkasının ürünü", "5023417", "4548736132610")
		for code, want := range map[string]string{"4548736132610": "Sony WH-1000XM5", "r9lmqs123456": "ASUS VG27AQ"} {
			got, err := s.ListProducts(ctx, a, store.ProductFilter{Code: code})
			if err != nil || len(got) != 1 || got[0].Name != want {
				t.Fatalf("code %s: %v %+v", code, err, got)
			}
			if got[0].Barcode == "" && want == "Sony WH-1000XM5" {
				t.Fatal("barcode not persisted")
			}
		}
		if got, _ := s.ListProducts(ctx, a, store.ProductFilter{Code: "4548"}); len(got) != 0 {
			t.Fatalf("code must match exactly, got %d", len(got))
		}
		if got, _ := s.ListProducts(ctx, a, store.ProductFilter{Query: "lmqs"}); len(got) != 1 {
			t.Fatalf("free-text search should include serial numbers, got %d", len(got))
		}
	})

	t.Run("products_scoped_and_cascade", func(t *testing.T) {
		s := newStore(t)
		a := mkUser(t, s, "a@x.io")
		b := mkUser(t, s, "b@x.io")
		p := &domain.Product{UserID: a, Name: "ASUS TUF VG27AQ", Brand: "ASUS", Category: "monitor", Seller: "MediaMarkt",
			PurchaseDate: domain.NewDate(2026, 9, 12), Price: 12499, Currency: "TRY", WarrantyMonths: 24}
		p.Normalize()
		if err := s.CreateProduct(ctx, p); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetProduct(ctx, a, p.ID)
		if err != nil || got.WarrantyEnd.String() != "2028-09-12" || got.Price != 12499 || got.Seller != "MediaMarkt" {
			t.Fatalf("get: %v %+v", err, got)
		}
		if _, err := s.GetProduct(ctx, b, p.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("other user must not see product")
		}
		p2 := &domain.Product{UserID: a, Name: "Sony WH-1000XM5", Brand: "Sony", Category: "headphones",
			PurchaseDate: domain.NewDate(2025, 1, 10), WarrantyMonths: 24, Currency: "TRY"}
		p2.Normalize()
		_ = s.CreateProduct(ctx, p2)

		all, _ := s.ListProducts(ctx, a, store.ProductFilter{})
		if len(all) != 2 {
			t.Fatalf("list %d", len(all))
		}
		mon, _ := s.ListProducts(ctx, a, store.ProductFilter{Category: "monitor"})
		if len(mon) != 1 || mon[0].ID != p.ID {
			t.Fatalf("category filter %v", mon)
		}
		q, _ := s.ListProducts(ctx, a, store.ProductFilter{Query: "sony"})
		if len(q) != 1 || q[0].ID != p2.ID {
			t.Fatalf("query filter %v", q)
		}
		win, _ := s.ListProducts(ctx, a, store.ProductFilter{EndFrom: domain.NewDate(2026, 10, 1), EndTo: domain.NewDate(2027, 2, 1)})
		if len(win) != 1 || win[0].ID != p2.ID {
			t.Fatalf("end window filter %v", win)
		}
		none, _ := s.ListProducts(ctx, b, store.ProductFilter{})
		if len(none) != 0 {
			t.Fatal("b sees a's products")
		}

		p.Name = "ASUS TUF Gaming VG27AQ"
		p.WarrantyMonths = 36
		p.Normalize()
		if err := s.UpdateProduct(ctx, p); err != nil {
			t.Fatal(err)
		}
		got, _ = s.GetProduct(ctx, a, p.ID)
		if got.Name != "ASUS TUF Gaming VG27AQ" || got.WarrantyEnd.String() != "2029-09-12" {
			t.Fatalf("update: %+v", got)
		}
		stolen := *p
		stolen.UserID = b
		if err := s.UpdateProduct(ctx, &stolen); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("cross-user update allowed")
		}

		doc := &domain.Document{UserID: a, ProductID: p.ID, Type: domain.DocInvoice, FileKey: "k1", ContentType: "image/jpeg", SizeBytes: 10, OriginalName: "fis.jpg"}
		if err := s.CreateDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
		if err := s.CreateDocument(ctx, &domain.Document{UserID: b, ProductID: p.ID, Type: domain.DocInvoice, FileKey: "k2"}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("document attached to other user's product")
		}
		docs, _ := s.ListDocuments(ctx, a, p.ID)
		if len(docs) != 1 || docs[0].OriginalName != "fis.jpg" {
			t.Fatalf("docs %v", docs)
		}
		if _, err := s.GetDocument(ctx, b, doc.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("b can read a's doc")
		}

		sr := &domain.ServiceRecord{UserID: a, ProductID: p.ID, Problem: domain.ProblemScreenLines, Status: domain.ServiceOpen, UnderWarranty: true}
		if err := s.CreateServiceRecord(ctx, sr); err != nil {
			t.Fatal(err)
		}
		sr.Status = domain.ServiceResolved
		sr.ServiceName = "ASUS Yetkili Servis"
		if err := s.UpdateServiceRecord(ctx, sr); err != nil {
			t.Fatal(err)
		}
		got2, _ := s.GetServiceRecord(ctx, a, sr.ID)
		if got2.Status != domain.ServiceResolved || got2.ServiceName != "ASUS Yetkili Servis" || !got2.UnderWarranty {
			t.Fatalf("service %+v", got2)
		}
		allSR, _ := s.ListServiceRecords(ctx, a, "")
		if len(allSR) != 1 {
			t.Fatal("list service records")
		}

		if err := s.DeleteProduct(ctx, b, p.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("b deleted a's product")
		}
		if err := s.DeleteProduct(ctx, a, p.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetDocument(ctx, a, doc.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("documents not cascaded")
		}
		if _, err := s.GetServiceRecord(ctx, a, sr.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("service records not cascaded")
		}
	})

	t.Run("service_history", func(t *testing.T) {
		s := newStore(t)
		a := mkUser(t, s, "sh@x.io")
		b := mkUser(t, s, "sh2@x.io")
		p := &domain.Product{UserID: a, Name: "TV", PurchaseDate: domain.NewDate(2026, 1, 1), WarrantyMonths: 24, Currency: "TRY"}
		if err := s.CreateProduct(ctx, p); err != nil {
			t.Fatal(err)
		}
		sr := &domain.ServiceRecord{UserID: a, ProductID: p.ID, Problem: domain.ProblemOther, Status: domain.ServiceOpen}
		if err := s.CreateServiceRecord(ctx, sr); err != nil {
			t.Fatal(err)
		}
		resolved := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
		sr.Status, sr.Cost, sr.ResolvedAt = domain.ServiceResolved, 1250.5, &resolved
		if err := s.UpdateServiceRecord(ctx, sr); err != nil {
			t.Fatal(err)
		}
		got, _ := s.GetServiceRecord(ctx, a, sr.ID)
		if got.Cost != 1250.5 || got.ResolvedAt == nil || !got.ResolvedAt.Equal(resolved) {
			t.Fatalf("cost/resolved_at %+v", got)
		}
		for _, st := range []domain.ServiceStatus{domain.ServiceOpen, domain.ServiceInService} {
			if err := s.AddServiceEvent(ctx, a, &domain.ServiceEvent{ServiceRecordID: sr.ID, Status: st, Note: string(st)}); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.AddServiceEvent(ctx, b, &domain.ServiceEvent{ServiceRecordID: sr.ID, Status: domain.ServiceOpen}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("b added an event to a's record")
		}
		evs, err := s.ListServiceEvents(ctx, a, sr.ID)
		if err != nil || len(evs) != 2 || evs[0].Status != domain.ServiceOpen || evs[1].Note != "in_service" || evs[0].ID == "" {
			t.Fatalf("events %v %v", evs, err)
		}
		if _, err := s.ListServiceEvents(ctx, b, sr.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("b listed a's events")
		}
		past := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC) // imported events keep their time
		if err := s.AddServiceEvent(ctx, a, &domain.ServiceEvent{ServiceRecordID: sr.ID, Status: domain.ServiceOpen, CreatedAt: past}); err != nil {
			t.Fatal(err)
		}
		if evs, _ := s.ListServiceEvents(ctx, a, sr.ID); len(evs) != 3 || !evs[0].CreatedAt.Equal(past) {
			t.Fatalf("given timestamp not kept %+v", evs)
		}
		doc := &domain.Document{UserID: a, ProductID: p.ID, ServiceRecordID: sr.ID, Type: domain.DocPhoto, FileKey: "k"}
		if err := s.CreateDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteServiceRecord(ctx, b, sr.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("b deleted a's record")
		}
		if err := s.DeleteServiceRecord(ctx, a, sr.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetServiceRecord(ctx, a, sr.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("record not deleted")
		}
		if d, err := s.GetDocument(ctx, a, doc.ID); err != nil || d.ServiceRecordID != "" {
			t.Fatalf("document should survive unlinked: %+v %v", d, err)
		}
	})

	t.Run("notifications", func(t *testing.T) {
		s := newStore(t)
		a := mkUser(t, s, "n@x.io")
		p := &domain.Product{UserID: a, Name: "X", PurchaseDate: domain.NewDate(2026, 1, 1), WarrantyMonths: 12, Currency: "TRY"}
		p.Normalize()
		_ = s.CreateProduct(ctx, p)
		base := time.Date(2026, 10, 1, 7, 0, 0, 0, time.UTC)
		ns := []domain.Notification{
			{Type: "warranty_90", ScheduledAt: base, Message: "90"},
			{Type: "warranty_30", ScheduledAt: base.Add(60 * 24 * time.Hour), Message: "30"},
		}
		if err := s.ReplaceNotifications(ctx, a, p.ID, ns); err != nil {
			t.Fatal(err)
		}
		due, _ := s.DueNotifications(ctx, base.Add(time.Hour), 10)
		if len(due) != 1 || due[0].Type != "warranty_90" {
			t.Fatalf("due %v", due)
		}
		if err := s.MarkNotificationSent(ctx, due[0].ID, base.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		due, _ = s.DueNotifications(ctx, base.Add(time.Hour), 10)
		if len(due) != 0 {
			t.Fatal("sent notification still due")
		}
		// replacing keeps already-sent reminders and does not duplicate them
		if err := s.ReplaceNotifications(ctx, a, p.ID, ns); err != nil {
			t.Fatal(err)
		}
		list, _ := s.ListNotifications(ctx, a)
		if len(list) != 2 {
			t.Fatalf("after replace %d", len(list))
		}
		sent := 0
		for _, n := range list {
			if n.Sent {
				sent++
			}
		}
		if sent != 1 {
			t.Fatalf("sent=%d", sent)
		}
	})

	t.Run("logs_and_categories", func(t *testing.T) {
		s := newStore(t)
		a := mkUser(t, s, "l@x.io")
		_ = s.CreateExtractionLog(ctx, &domain.ExtractionLog{UserID: a, Provider: "gemini", Model: "gemini-3.5-flash", Success: true})
		_ = s.CreateExtractionLog(ctx, &domain.ExtractionLog{UserID: a, Provider: "rules", Success: false, Error: "quota"})
		fails, _ := s.ListExtractionLogs(ctx, true, 10)
		if len(fails) != 1 || fails[0].Error != "quota" {
			t.Fatalf("fails %v", fails)
		}
		if err := s.CreateExtractionLog(ctx, &domain.ExtractionLog{UserID: a, Kind: domain.LogDiagnosis, Provider: "gemini", Model: "m", Success: true}); err != nil {
			t.Fatal(err)
		}
		all, _ := s.ListExtractionLogs(ctx, false, 10)
		kinds := map[string]int{}
		for _, l := range all {
			kinds[l.Kind]++
			if l.Provider == "gemini" && l.Kind == domain.LogReceipt && l.Model != "gemini-3.5-flash" {
				t.Fatalf("model not stored: %+v", l)
			}
		}
		if kinds[domain.LogReceipt] != 2 || kinds[domain.LogDiagnosis] != 1 {
			t.Fatalf("kinds %v", kinds)
		}
		if len(all) != 3 {
			t.Fatal("all logs")
		}
		cats, _ := s.ListCategories(ctx)
		if len(cats) != len(domain.DefaultCategories) || cats[len(cats)-1].Slug != "other" {
			t.Fatalf("cats %v", cats)
		}
		_ = s.UpsertCategory(ctx, domain.Category{Slug: "drone", NameTR: "Drone"})
		if err := s.DeleteCategory(ctx, "drone"); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteCategory(ctx, "drone"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("double delete")
		}
	})
}

func mkUser(t *testing.T, s store.Store, email string) string {
	t.Helper()
	u := &domain.User{Email: email, Name: "T", PasswordHash: "h"}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u.ID
}
