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
		_ = s.CreateExtractionLog(ctx, &domain.ExtractionLog{UserID: a, Provider: "gemini", Success: true})
		_ = s.CreateExtractionLog(ctx, &domain.ExtractionLog{UserID: a, Provider: "rules", Success: false, Error: "quota"})
		fails, _ := s.ListExtractionLogs(ctx, true, 10)
		if len(fails) != 1 || fails[0].Error != "quota" {
			t.Fatalf("fails %v", fails)
		}
		all, _ := s.ListExtractionLogs(ctx, false, 10)
		if len(all) != 2 {
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
