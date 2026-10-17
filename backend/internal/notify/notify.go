// Package notify plans warranty-expiry reminders (90/30/7 days before) and runs a worker
// that marks due reminders as sent. The device also schedules local notifications; when FCM is
// configured, Worker.Send is push.Notifier.Deliver.
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store"
)

// ReminderHour is the local hour reminders fire at.
const ReminderHour = 10

// Location for reminder scheduling (Türkiye has a fixed UTC+3 offset).
var Location = time.FixedZone("TRT", 3*60*60)

// TypeFor returns the notification type for an offset.
func TypeFor(days int) domain.NotificationType {
	return domain.NotificationType(fmt.Sprintf("warranty_%d", days))
}

// Message builds the Turkish reminder text, e.g.
// "ASUS monitörünüzün garantisinin bitmesine 30 gün kaldı."
func Message(p domain.Product, days int) string {
	return fmt.Sprintf("%s garantisinin bitmesine %d gün kaldı.", Possessive(p), days)
}

// categoryPossessive holds hand-written Turkish possessive+genitive forms ("-ünüzün"),
// including consonant softening (kulaklık → kulaklığınızın) which a naive suffixer gets wrong.
var categoryPossessive = map[string]string{
	"monitor": "monitörünüzün", "laptop": "laptopunuzun", "phone": "telefonunuzun", "headphones": "kulaklığınızın",
	"tablet": "tabletinizin", "tv": "televizyonunuzun", "appliance": "cihazınızın", "small_appliance": "cihazınızın",
	"camera": "kameranızın", "console": "konsolunuzun", "wearable": "saatinizin",
}

// Possessive returns e.g. "ASUS monitörünüzün" or "Dedica EC685 ürününüzün".
func Possessive(p domain.Product) string {
	if form, ok := categoryPossessive[p.Category]; ok && p.Brand != "" {
		return p.Brand + " " + form
	}
	if p.Name == "" {
		return "Ürününüzün"
	}
	return p.Name + " ürününüzün"
}

// Plan returns reminders for a product that are still in the future relative to now.
func Plan(p domain.Product, now time.Time) []domain.Notification {
	if p.WarrantyEnd.IsZero() {
		return nil
	}
	var out []domain.Notification
	for _, days := range domain.ReminderOffsets {
		day := p.WarrantyEnd.AddDays(-days).Time()
		at := time.Date(day.Year(), day.Month(), day.Day(), ReminderHour, 0, 0, 0, Location)
		if !at.After(now) {
			continue
		}
		out = append(out, domain.Notification{
			ProductID: p.ID, UserID: p.UserID, Type: TypeFor(days), Message: Message(p, days), ScheduledAt: at.UTC(),
		})
	}
	return out
}

// Sync (re)plans reminders for a product.
func Sync(ctx context.Context, s store.Store, p domain.Product, now time.Time) error {
	return s.ReplaceNotifications(ctx, p.UserID, p.ID, Plan(p, now))
}

// Sender delivers a reminder (FCM push). nil → just mark as sent.
type Sender func(ctx context.Context, n domain.Notification) error

// Worker periodically processes due reminders.
type Worker struct {
	Store    store.Store
	Send     Sender
	Interval time.Duration
	Logger   *slog.Logger
	Now      func() time.Time
}

// RunOnce processes all due reminders and returns how many were sent.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	now := time.Now()
	if w.Now != nil {
		now = w.Now()
	}
	due, err := w.Store.DueNotifications(ctx, now, 200)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, n := range due {
		if w.Send != nil {
			if err := w.Send(ctx, n); err != nil {
				if w.Logger != nil {
					w.Logger.Warn("reminder delivery failed", "id", n.ID, "err", err)
				}
				continue
			}
		}
		if err := w.Store.MarkNotificationSent(ctx, n.ID, now); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

// Run loops until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if n, err := w.RunOnce(ctx); err != nil && w.Logger != nil {
			w.Logger.Error("notification worker", "err", err)
		} else if n > 0 && w.Logger != nil {
			w.Logger.Info("reminders processed", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
