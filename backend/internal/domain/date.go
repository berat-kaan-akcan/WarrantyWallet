// Package domain holds the pure business model of Warranty Wallet.
// It has no I/O and no dependencies on other internal packages.
package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DateLayout is the wire format for calendar dates (ISO-8601, no time).
const DateLayout = "2006-01-02"

// Date is a calendar date without a time-of-day, always normalised to UTC midnight.
type Date struct{ t time.Time }

// NewDate builds a Date from year, month and day.
func NewDate(y int, m time.Month, d int) Date {
	return Date{time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

// DateOf truncates a timestamp to its calendar date in the given location's view.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return NewDate(y, m, d)
}

// ParseDate parses "YYYY-MM-DD".
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(DateLayout, strings.TrimSpace(s))
	if err != nil {
		return Date{}, fmt.Errorf("invalid date %q: want YYYY-MM-DD", s)
	}
	return Date{t}, nil
}

// IsZero reports whether the date is unset.
func (d Date) IsZero() bool { return d.t.IsZero() }

// Time returns the date as UTC midnight.
func (d Date) Time() time.Time { return d.t }

// String formats as YYYY-MM-DD ("" for zero).
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.t.Format(DateLayout)
}

// Before reports whether d is strictly before o.
func (d Date) Before(o Date) bool { return d.t.Before(o.t) }

// After reports whether d is strictly after o.
func (d Date) After(o Date) bool { return d.t.After(o.t) }

// DaysUntil returns the whole days from d to o (negative if o is earlier).
func (d Date) DaysUntil(o Date) int {
	return int(o.t.Sub(d.t).Hours() / 24)
}

// AddDays adds n days.
func (d Date) AddDays(n int) Date { return Date{d.t.AddDate(0, 0, n)} }

// MarshalJSON encodes as "YYYY-MM-DD" or null.
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(d.String())
}

// UnmarshalJSON accepts "YYYY-MM-DD", "" or null.
func (d *Date) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*d = Date{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		*d = Date{}
		return nil
	}
	p, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = p
	return nil
}

// Value implements driver.Valuer.
func (d Date) Value() (driver.Value, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.t, nil
}

// Scan implements sql.Scanner.
func (d *Date) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*d = Date{}
	case time.Time:
		*d = DateOf(v)
	case string:
		p, err := ParseDate(v[:min(len(v), 10)])
		if err != nil {
			return err
		}
		*d = p
	case []byte:
		return d.Scan(string(v))
	default:
		return fmt.Errorf("cannot scan %T into Date", src)
	}
	return nil
}
