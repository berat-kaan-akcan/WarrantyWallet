package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store/memory"
)

func TestMessage(t *testing.T) {
	cases := []struct {
		p    domain.Product
		want string
	}{
		{domain.Product{Brand: "ASUS", Category: "monitor"}, "ASUS monitörünüzün garantisinin bitmesine 30 gün kaldı."},
		{domain.Product{Brand: "Apple", Category: "laptop"}, "Apple laptopunuzun garantisinin bitmesine 30 gün kaldı."},
		{domain.Product{Brand: "Sony", Category: "headphones"}, "Sony kulaklığınızın garantisinin bitmesine 30 gün kaldı."},
		{domain.Product{Brand: "Samsung", Category: "phone"}, "Samsung telefonunuzun garantisinin bitmesine 30 gün kaldı."},
		{domain.Product{Name: "Dedica EC685", Category: "other"}, "Dedica EC685 ürününüzün garantisinin bitmesine 30 gün kaldı."},
	}
	for _, c := range cases {
		if got := Message(c.p, 30); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
}

func TestPlan(t *testing.T) {
	p := domain.Product{ID: "p", UserID: "u", Brand: "ASUS", Category: "monitor", PurchaseDate: domain.NewDate(2026, 9, 12), WarrantyMonths: 24}
	p.Normalize()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	ns := Plan(p, now)
	if len(ns) != 3 {
		t.Fatalf("want 3 reminders, got %d", len(ns))
	}
	// 2028-09-12 minus 90 days = 2028-06-14, 10:00 TRT = 07:00 UTC
	if ns[0].Type != "warranty_90" || !ns[0].ScheduledAt.Equal(time.Date(2028, 6, 14, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("first %+v", ns[0])
	}
	if ns[2].Type != "warranty_7" || ns[2].ScheduledAt.Format("2006-01-02") != "2028-09-05" {
		t.Fatalf("last %+v", ns[2])
	}
	// Only the future ones are planned: 20 days before end → only 7-day reminder left.
	late := p.WarrantyEnd.AddDays(-20).Time()
	if got := Plan(p, late); len(got) != 1 || got[0].Type != "warranty_7" {
		t.Fatalf("late plan %+v", got)
	}
	if Plan(domain.Product{}, now) != nil {
		t.Fatal("no end date → no reminders")
	}
}

func TestWorker(t *testing.T) {
	ctx := context.Background()
	s := memory.New()
	u := &domain.User{Email: "w@x.io", PasswordHash: "h"}
	_ = s.CreateUser(ctx, u)
	p := &domain.Product{UserID: u.ID, Name: "X", Brand: "ASUS", Category: "monitor", PurchaseDate: domain.NewDate(2026, 1, 10), WarrantyMonths: 12}
	p.Normalize()
	_ = s.CreateProduct(ctx, p)
	if err := Sync(ctx, s, *p, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	var delivered []string
	fail := true
	w := &Worker{Store: s, Now: func() time.Time { return time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC) },
		Send: func(_ context.Context, n domain.Notification) error {
			if fail && n.Type == "warranty_30" {
				fail = false
				return errors.New("push down")
			}
			delivered = append(delivered, string(n.Type))
			return nil
		}}
	n, err := w.RunOnce(ctx) // 90 and 30 are due (end 2027-01-10); 30 fails once
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	n, _ = w.RunOnce(ctx) // retried
	if n != 1 || len(delivered) != 2 {
		t.Fatalf("retry n=%d delivered=%v", n, delivered)
	}
	n, _ = w.RunOnce(ctx)
	if n != 0 {
		t.Fatal("nothing should be due")
	}
}
