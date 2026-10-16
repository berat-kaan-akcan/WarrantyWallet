package domain

import "time"

// DefaultWarrantyMonths is suggested when a document does not state a warranty period
// (2 years is the statutory minimum for new goods under Turkish consumer law).
const DefaultWarrantyMonths = 24

// ExpiringSoonDays is the threshold for the "Garanti yakında bitiyor" status.
const ExpiringSoonDays = 30

// ReminderOffsets are the days-before-expiry at which reminders fire.
var ReminderOffsets = []int{90, 30, 7}

// WarrantyStatus is derived, never stored.
type WarrantyStatus string

// Warranty statuses.
const (
	StatusActive       WarrantyStatus = "active"
	StatusExpiringSoon WarrantyStatus = "expiring_soon"
	StatusExpired      WarrantyStatus = "expired"
	StatusUnknown      WarrantyStatus = "unknown"
)

// AddMonths adds n calendar months, clamping to the last day of the target month
// (31 Jan + 1 month = 28/29 Feb), unlike time.AddDate which overflows into March.
func AddMonths(d Date, n int) Date {
	if d.IsZero() {
		return d
	}
	y, m, day := d.t.Date()
	total := int(m) - 1 + n
	ty := y + floorDiv(total, 12)
	tm := time.Month(floorMod(total, 12) + 1)
	last := daysIn(ty, tm)
	if day > last {
		day = last
	}
	return NewDate(ty, tm, day)
}

// WarrantyEnd computes the warranty end date. Zero purchase date or non-positive months → zero date.
func WarrantyEnd(purchase Date, months int) Date {
	if purchase.IsZero() || months <= 0 {
		return Date{}
	}
	return AddMonths(purchase, months)
}

// DaysLeft returns whole days from today until end (negative once expired).
func DaysLeft(end, today Date) int {
	return today.DaysUntil(end)
}

// StatusOf derives the status on `today`. The end date itself is still covered.
func StatusOf(end, today Date) WarrantyStatus {
	if end.IsZero() {
		return StatusUnknown
	}
	left := DaysLeft(end, today)
	switch {
	case left < 0:
		return StatusExpired
	case left <= ExpiringSoonDays:
		return StatusExpiringSoon
	default:
		return StatusActive
	}
}

// IsCovered reports whether the warranty is still valid on `today`.
func IsCovered(end, today Date) bool {
	s := StatusOf(end, today)
	return s == StatusActive || s == StatusExpiringSoon
}

func daysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func floorMod(a, b int) int { return a - floorDiv(a, b)*b }
