package domain

import (
	"strings"
	"time"
)

// ServiceCenter is an authorized service point for a brand, maintained by admins.
type ServiceCenter struct {
	ID        string    `json:"id"`
	Brand     string    `json:"brand"`
	Name      string    `json:"name"`
	City      string    `json:"city"`
	District  string    `json:"district"`
	Address   string    `json:"address"`
	Phone     string    `json:"phone"`
	Website   string    `json:"website"`
	Lat       *float64  `json:"lat"`
	Lng       *float64  `json:"lng"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var foldReplacer = strings.NewReplacer(
	"İ", "i", "I", "i", "ı", "i", "Ş", "s", "ş", "s", "Ğ", "g", "ğ", "g",
	"Ü", "u", "ü", "u", "Ö", "o", "ö", "o", "Ç", "c", "ç", "c",
)

// FoldKey normalizes a brand or city for matching: Turkish letters folded to ASCII, lower case,
// single spaces. "İSTANBUL", "Istanbul" and "istanbul" all become "istanbul".
func FoldKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(foldReplacer.Replace(s))), " ")
}
