package domain

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Sentinel errors shared by stores and services.
var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrInvalidInput  = errors.New("invalid input")
	ErrUnprocessable = errors.New("unprocessable")
)

// Role of a user.
type Role string

// Roles.
const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// User is an account.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         Role      `json:"role"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Product is a purchased item with its warranty.
type Product struct {
	ID             string    `json:"id"`
	UserID         string    `json:"-"`
	Name           string    `json:"name"`
	Brand          string    `json:"brand"`
	Model          string    `json:"model"`
	Category       string    `json:"category"`
	Seller         string    `json:"seller"`
	PurchaseDate   Date      `json:"purchase_date"`
	Price          float64   `json:"price"`
	Currency       string    `json:"currency"`
	WarrantyMonths int       `json:"warranty_months"`
	WarrantyEnd    Date      `json:"warranty_end"`
	SerialNumber   string    `json:"serial_number"`
	Barcode        string    `json:"barcode"`
	Notes          string    `json:"notes"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Normalize trims fields, applies defaults and recomputes the warranty end.
// The end date is always derived server-side and never trusted from clients.
func (p *Product) Normalize() {
	p.Name = strings.TrimSpace(p.Name)
	p.Brand = strings.TrimSpace(p.Brand)
	p.Model = strings.TrimSpace(p.Model)
	p.Category = strings.ToLower(strings.TrimSpace(p.Category))
	if p.Category == "" {
		p.Category = "other"
	}
	p.Seller = strings.TrimSpace(p.Seller)
	p.SerialNumber = strings.TrimSpace(p.SerialNumber)
	p.Barcode = strings.TrimSpace(p.Barcode)
	p.Notes = strings.TrimSpace(p.Notes)
	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	if p.Currency == "" {
		p.Currency = "TRY"
	}
	p.WarrantyEnd = WarrantyEnd(p.PurchaseDate, p.WarrantyMonths)
}

// MaxPrice keeps prices inside numeric(12,2) with a wide safety margin.
const MaxPrice = 1_000_000_000

// MinPurchaseDate rejects obviously wrong years (e.g. OCR reading "0026").
var MinPurchaseDate = NewDate(1990, 1, 1)

// Validate returns field errors (empty map = valid). Lengths count characters, not bytes,
// so Turkish letters are not penalised.
func (p *Product) Validate(today Date) map[string]string {
	errs := map[string]string{}
	if p.Name == "" {
		errs["name"] = "required"
	} else if utf8.RuneCountInString(p.Name) > 200 {
		errs["name"] = "max 200 characters"
	}
	switch {
	case p.PurchaseDate.IsZero():
		errs["purchase_date"] = "required"
	case p.PurchaseDate.After(today.AddDays(1)):
		errs["purchase_date"] = "cannot be in the future"
	case p.PurchaseDate.Before(MinPurchaseDate):
		errs["purchase_date"] = "must be 1990-01-01 or later"
	}
	if p.WarrantyMonths < 0 || p.WarrantyMonths > 240 {
		errs["warranty_months"] = "must be between 0 and 240"
	}
	if p.Price < 0 {
		errs["price"] = "cannot be negative"
	} else if p.Price > MaxPrice {
		errs["price"] = "too large"
	}
	if !isCurrencyCode(p.Currency) {
		errs["currency"] = "must be a 3-letter ISO code"
	}
	for k, v := range map[string]string{"brand": p.Brand, "model": p.Model, "seller": p.Seller, "serial_number": p.SerialNumber, "category": p.Category} {
		if utf8.RuneCountInString(v) > 120 {
			errs[k] = "max 120 characters"
		}
	}
	if utf8.RuneCountInString(p.Barcode) > 128 {
		errs["barcode"] = "max 128 characters"
	}
	if utf8.RuneCountInString(p.Notes) > 2000 {
		errs["notes"] = "max 2000 characters"
	}
	return errs
}

func isCurrencyCode(c string) bool {
	if len(c) != 3 {
		return false
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// DocumentType classifies stored files.
type DocumentType string

// Document types.
const (
	DocInvoice  DocumentType = "invoice"
	DocWarranty DocumentType = "warranty"
	DocService  DocumentType = "service"
	DocPhoto    DocumentType = "photo"
)

// ValidDocumentType reports whether t is known.
func ValidDocumentType(t DocumentType) bool {
	switch t {
	case DocInvoice, DocWarranty, DocService, DocPhoto:
		return true
	}
	return false
}

// Document is a stored file (invoice, warranty card, service form, photo).
type Document struct {
	ID              string       `json:"id"`
	UserID          string       `json:"-"`
	ProductID       string       `json:"product_id"`
	ServiceRecordID string       `json:"service_record_id,omitempty"`
	Type            DocumentType `json:"type"`
	FileKey         string       `json:"-"`
	ContentType     string       `json:"content_type"`
	SizeBytes       int64        `json:"size_bytes"`
	OriginalName    string       `json:"original_name"`
	CreatedAt       time.Time    `json:"created_at"`
}

// ProblemType for a broken product.
type ProblemType string

// Problem types ("Ürünüm Arızalandı").
const (
	ProblemWontTurnOn     ProblemType = "wont_turn_on"
	ProblemScreenLines    ProblemType = "screen_lines"
	ProblemPhysicalDamage ProblemType = "physical_damage"
	ProblemOther          ProblemType = "other"
)

// ValidProblem reports whether p is known.
func ValidProblem(p ProblemType) bool {
	switch p {
	case ProblemWontTurnOn, ProblemScreenLines, ProblemPhysicalDamage, ProblemOther:
		return true
	}
	return false
}

// ServiceStatus of a repair.
type ServiceStatus string

// Service statuses.
const (
	ServiceOpen      ServiceStatus = "open"
	ServiceInService ServiceStatus = "in_service"
	ServiceResolved  ServiceStatus = "resolved"
	ServiceCancelled ServiceStatus = "cancelled"
)

// ValidServiceStatus reports whether s is known.
func ValidServiceStatus(s ServiceStatus) bool {
	switch s {
	case ServiceOpen, ServiceInService, ServiceResolved, ServiceCancelled:
		return true
	}
	return false
}

// ServiceRecord is a repair/service case.
type ServiceRecord struct {
	ID            string        `json:"id"`
	UserID        string        `json:"-"`
	ProductID     string        `json:"product_id"`
	Problem       ProblemType   `json:"problem"`
	Description   string        `json:"description"`
	ServiceName   string        `json:"service_name"`
	ServiceDate   Date          `json:"service_date"`
	Status        ServiceStatus `json:"status"`
	Notes         string        `json:"notes"`
	UnderWarranty bool          `json:"under_warranty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// NotificationType identifies a reminder.
type NotificationType string

// Notification is a scheduled warranty reminder.
type Notification struct {
	ID          string           `json:"id"`
	UserID      string           `json:"-"`
	ProductID   string           `json:"product_id"`
	Type        NotificationType `json:"type"`
	Message     string           `json:"message"`
	ScheduledAt time.Time        `json:"scheduled_at"`
	Sent        bool             `json:"sent"`
	SentAt      *time.Time       `json:"sent_at,omitempty"`
}

// ExtractionLog records each receipt extraction attempt (admin "OCR errors" view).
type ExtractionLog struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Provider  string    `json:"provider"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	RawText   string    `json:"raw_text,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Category is an admin-managed product category.
type Category struct {
	Slug   string `json:"slug"`
	NameTR string `json:"name_tr"`
}

// DefaultCategories seeds the category list.
var DefaultCategories = []Category{
	{"phone", "Telefon"}, {"laptop", "Laptop"}, {"monitor", "Monitör"}, {"tv", "Televizyon"},
	{"headphones", "Kulaklık"}, {"tablet", "Tablet"}, {"appliance", "Beyaz Eşya"},
	{"small_appliance", "Küçük Ev Aleti"}, {"camera", "Kamera"}, {"console", "Oyun Konsolu"},
	{"wearable", "Akıllı Saat"}, {"other", "Diğer"},
}

// NewID returns a random RFC 4122 v4 UUID.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// AuthTokenKind distinguishes opaque server-side tokens.
type AuthTokenKind string

// Auth token kinds.
const (
	TokenRefresh       AuthTokenKind = "refresh"
	TokenPasswordReset AuthTokenKind = "password_reset"
)

// AuthToken is an opaque token (refresh or password reset). Only its SHA-256 hash is stored;
// the plaintext is shown to the client once.
type AuthToken struct {
	ID        string
	UserID    string
	Kind      AuthTokenKind
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}
