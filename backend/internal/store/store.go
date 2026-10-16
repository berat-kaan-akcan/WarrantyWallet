// Package store defines persistence interfaces. Implementations: memory (tests/dev) and
// postgres (production). Every user-owned query is scoped by userID; a resource owned by
// another user is reported as domain.ErrNotFound so its existence is not leaked.
package store

import (
	"context"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

// ProductFilter narrows product listings.
type ProductFilter struct {
	Category string
	Query    string // matches name/brand/model/seller/serial/barcode, case-insensitive
	Code     string // exact, case-insensitive match on barcode OR serial number (scanner lookup)
	EndFrom  domain.Date
	EndTo    domain.Date
}

// Store is the full persistence contract used by the HTTP layer.
type Store interface {
	CreateUser(ctx context.Context, u *domain.User) error // ErrConflict on duplicate email
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	GetUserByID(ctx context.Context, id string) (domain.User, error)
	ListUsers(ctx context.Context) ([]domain.User, error)
	SetUserRole(ctx context.Context, id string, role domain.Role) error
	DeleteUser(ctx context.Context, id string) error
	UpdateUserProfile(ctx context.Context, id, name string) error
	SetUserPassword(ctx context.Context, id, passwordHash string) error

	// Opaque auth tokens (refresh / password reset), stored hashed.
	CreateAuthToken(ctx context.Context, t *domain.AuthToken) error
	// ConsumeAuthToken atomically marks an unused, unexpired token as used and returns it;
	// ErrNotFound when it is unknown, already used or expired.
	ConsumeAuthToken(ctx context.Context, kind domain.AuthTokenKind, hash string, now time.Time) (domain.AuthToken, error)
	// RevokeAuthTokens marks all unused tokens of the kind for the user as used.
	RevokeAuthTokens(ctx context.Context, userID string, kind domain.AuthTokenKind, now time.Time) error

	CreateProduct(ctx context.Context, p *domain.Product) error
	GetProduct(ctx context.Context, userID, id string) (domain.Product, error)
	ListProducts(ctx context.Context, userID string, f ProductFilter) ([]domain.Product, error)
	UpdateProduct(ctx context.Context, p *domain.Product) error
	DeleteProduct(ctx context.Context, userID, id string) error

	CreateDocument(ctx context.Context, d *domain.Document) error
	GetDocument(ctx context.Context, userID, id string) (domain.Document, error)
	ListDocuments(ctx context.Context, userID, productID string) ([]domain.Document, error)
	DeleteDocument(ctx context.Context, userID, id string) error

	CreateServiceRecord(ctx context.Context, r *domain.ServiceRecord) error
	GetServiceRecord(ctx context.Context, userID, id string) (domain.ServiceRecord, error)
	ListServiceRecords(ctx context.Context, userID, productID string) ([]domain.ServiceRecord, error) // productID "" = all
	UpdateServiceRecord(ctx context.Context, r *domain.ServiceRecord) error

	ReplaceNotifications(ctx context.Context, userID, productID string, ns []domain.Notification) error
	ListNotifications(ctx context.Context, userID string) ([]domain.Notification, error)
	DueNotifications(ctx context.Context, now time.Time, limit int) ([]domain.Notification, error)
	MarkNotificationSent(ctx context.Context, id string, at time.Time) error

	CreateExtractionLog(ctx context.Context, l *domain.ExtractionLog) error
	ListExtractionLogs(ctx context.Context, onlyFailures bool, limit int) ([]domain.ExtractionLog, error)

	ListCategories(ctx context.Context) ([]domain.Category, error)
	UpsertCategory(ctx context.Context, c domain.Category) error
	DeleteCategory(ctx context.Context, slug string) error

	Ping(ctx context.Context) error
}
