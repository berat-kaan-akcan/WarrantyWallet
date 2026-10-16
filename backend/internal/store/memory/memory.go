// Package memory is a thread-safe in-memory Store used by tests and for running the API
// without PostgreSQL (DATABASE_URL unset). Data is lost on restart.
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store"
)

// Store implements store.Store in memory.
type Store struct {
	mu         sync.RWMutex
	users      map[string]domain.User
	products   map[string]domain.Product
	documents  map[string]domain.Document
	services   map[string]domain.ServiceRecord
	notifs     map[string]domain.Notification
	logs       []domain.ExtractionLog
	categories map[string]domain.Category
	tokens     map[string]domain.AuthToken // by hash
	now        func() time.Time
}

var _ store.Store = (*Store)(nil)

// New returns an empty store seeded with default categories.
func New() *Store {
	s := &Store{
		users: map[string]domain.User{}, products: map[string]domain.Product{},
		documents: map[string]domain.Document{}, services: map[string]domain.ServiceRecord{},
		notifs: map[string]domain.Notification{}, categories: map[string]domain.Category{},
		tokens: map[string]domain.AuthToken{},
		now:    time.Now,
	}
	for _, c := range domain.DefaultCategories {
		s.categories[c.Slug] = c
	}
	return s
}

// Ping always succeeds.
func (s *Store) Ping(context.Context) error { return nil }

// CreateUser stores a user; email is unique case-insensitively.
func (s *Store) CreateUser(_ context.Context, u *domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	for _, x := range s.users {
		if x.Email == u.Email {
			return domain.ErrConflict
		}
	}
	if u.ID == "" {
		u.ID = domain.NewID()
	}
	if u.Role == "" {
		u.Role = domain.RoleUser
	}
	u.CreatedAt = s.now().UTC()
	s.users[u.ID] = *u
	return nil
}

// GetUserByEmail looks up by email.
func (s *Store) GetUserByEmail(_ context.Context, email string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	email = strings.ToLower(strings.TrimSpace(email))
	for _, u := range s.users {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

// GetUserByID looks up by id.
func (s *Store) GetUserByID(_ context.Context, id string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}

// ListUsers returns all users ordered by creation.
func (s *Store) ListUsers(context.Context) ([]domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// SetUserRole changes a role.
func (s *Store) SetUserRole(_ context.Context, id string, role domain.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.Role = role
	s.users[id] = u
	return nil
}

// DeleteUser removes a user and all owned data.
func (s *Store) DeleteUser(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return domain.ErrNotFound
	}
	delete(s.users, id)
	for h, t := range s.tokens {
		if t.UserID == id {
			delete(s.tokens, h)
		}
	}
	for k, p := range s.products {
		if p.UserID == id {
			s.deleteProductLocked(k)
		}
	}
	return nil
}

// UpdateUserProfile changes the display name.
func (s *Store) UpdateUserProfile(_ context.Context, id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.Name = name
	s.users[id] = u
	return nil
}

// SetUserPassword replaces the password hash.
func (s *Store) SetUserPassword(_ context.Context, id, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.PasswordHash = hash
	s.users[id] = u
	return nil
}

// CreateAuthToken stores a hashed token.
func (s *Store) CreateAuthToken(_ context.Context, t *domain.AuthToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[t.UserID]; !ok {
		return domain.ErrNotFound
	}
	if _, ok := s.tokens[t.TokenHash]; ok {
		return domain.ErrConflict
	}
	if t.ID == "" {
		t.ID = domain.NewID()
	}
	t.CreatedAt = s.now().UTC()
	s.tokens[t.TokenHash] = *t
	return nil
}

// ConsumeAuthToken marks a valid token as used.
func (s *Store) ConsumeAuthToken(_ context.Context, kind domain.AuthTokenKind, hash string, now time.Time) (domain.AuthToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[hash]
	if !ok || t.Kind != kind || t.UsedAt != nil || !now.Before(t.ExpiresAt) {
		return domain.AuthToken{}, domain.ErrNotFound
	}
	at := now.UTC()
	t.UsedAt = &at
	s.tokens[hash] = t
	return t, nil
}

// RevokeAuthTokens marks all unused tokens of a kind as used.
func (s *Store) RevokeAuthTokens(_ context.Context, userID string, kind domain.AuthTokenKind, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	at := now.UTC()
	for h, t := range s.tokens {
		if t.UserID == userID && t.Kind == kind && t.UsedAt == nil {
			t.UsedAt = &at
			s.tokens[h] = t
		}
	}
	return nil
}

// CreateProduct stores a product.
func (s *Store) CreateProduct(_ context.Context, p *domain.Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = domain.NewID()
	}
	now := s.now().UTC()
	p.CreatedAt, p.UpdatedAt = now, now
	s.products[p.ID] = *p
	return nil
}

// GetProduct returns an owned product.
func (s *Store) GetProduct(_ context.Context, userID, id string) (domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.products[id]
	if !ok || p.UserID != userID {
		return domain.Product{}, domain.ErrNotFound
	}
	return p, nil
}

// ListProducts lists owned products, newest first.
func (s *Store) ListProducts(_ context.Context, userID string, f store.ProductFilter) ([]domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(strings.TrimSpace(f.Query))
	out := []domain.Product{}
	for _, p := range s.products {
		if p.UserID != userID {
			continue
		}
		if f.Category != "" && p.Category != f.Category {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(p.Name+" "+p.Brand+" "+p.Model+" "+p.Seller), q) {
			continue
		}
		if !f.EndFrom.IsZero() && (p.WarrantyEnd.IsZero() || p.WarrantyEnd.Before(f.EndFrom)) {
			continue
		}
		if !f.EndTo.IsZero() && (p.WarrantyEnd.IsZero() || p.WarrantyEnd.After(f.EndTo)) {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// UpdateProduct replaces an owned product.
func (s *Store) UpdateProduct(_ context.Context, p *domain.Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.products[p.ID]
	if !ok || old.UserID != p.UserID {
		return domain.ErrNotFound
	}
	p.CreatedAt = old.CreatedAt
	p.UpdatedAt = s.now().UTC()
	s.products[p.ID] = *p
	return nil
}

// DeleteProduct removes an owned product and its documents, service records and notifications.
func (s *Store) DeleteProduct(_ context.Context, userID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.products[id]
	if !ok || p.UserID != userID {
		return domain.ErrNotFound
	}
	s.deleteProductLocked(id)
	return nil
}

func (s *Store) deleteProductLocked(id string) {
	delete(s.products, id)
	for k, d := range s.documents {
		if d.ProductID == id {
			delete(s.documents, k)
		}
	}
	for k, r := range s.services {
		if r.ProductID == id {
			delete(s.services, k)
		}
	}
	for k, n := range s.notifs {
		if n.ProductID == id {
			delete(s.notifs, k)
		}
	}
}

// CreateDocument stores document metadata.
func (s *Store) CreateDocument(_ context.Context, d *domain.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.products[d.ProductID]; !ok || p.UserID != d.UserID {
		return domain.ErrNotFound
	}
	if d.ID == "" {
		d.ID = domain.NewID()
	}
	d.CreatedAt = s.now().UTC()
	s.documents[d.ID] = *d
	return nil
}

// GetDocument returns an owned document.
func (s *Store) GetDocument(_ context.Context, userID, id string) (domain.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.documents[id]
	if !ok || d.UserID != userID {
		return domain.Document{}, domain.ErrNotFound
	}
	return d, nil
}

// ListDocuments lists documents of an owned product, oldest first.
func (s *Store) ListDocuments(_ context.Context, userID, productID string) ([]domain.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Document{}
	for _, d := range s.documents {
		if d.UserID == userID && d.ProductID == productID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// DeleteDocument removes an owned document.
func (s *Store) DeleteDocument(_ context.Context, userID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.documents[id]
	if !ok || d.UserID != userID {
		return domain.ErrNotFound
	}
	delete(s.documents, id)
	return nil
}

// CreateServiceRecord stores a service record.
func (s *Store) CreateServiceRecord(_ context.Context, r *domain.ServiceRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.products[r.ProductID]; !ok || p.UserID != r.UserID {
		return domain.ErrNotFound
	}
	if r.ID == "" {
		r.ID = domain.NewID()
	}
	now := s.now().UTC()
	r.CreatedAt, r.UpdatedAt = now, now
	s.services[r.ID] = *r
	return nil
}

// GetServiceRecord returns an owned record.
func (s *Store) GetServiceRecord(_ context.Context, userID, id string) (domain.ServiceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.services[id]
	if !ok || r.UserID != userID {
		return domain.ServiceRecord{}, domain.ErrNotFound
	}
	return r, nil
}

// ListServiceRecords lists owned records, newest first.
func (s *Store) ListServiceRecords(_ context.Context, userID, productID string) ([]domain.ServiceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.ServiceRecord{}
	for _, r := range s.services {
		if r.UserID == userID && (productID == "" || r.ProductID == productID) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// UpdateServiceRecord replaces an owned record.
func (s *Store) UpdateServiceRecord(_ context.Context, r *domain.ServiceRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.services[r.ID]
	if !ok || old.UserID != r.UserID {
		return domain.ErrNotFound
	}
	r.CreatedAt = old.CreatedAt
	r.UpdatedAt = s.now().UTC()
	s.services[r.ID] = *r
	return nil
}

// ReplaceNotifications replaces a product's pending reminders (sent ones are kept).
func (s *Store) ReplaceNotifications(_ context.Context, userID, productID string, ns []domain.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sent := map[domain.NotificationType]bool{}
	for k, n := range s.notifs {
		if n.ProductID == productID {
			if n.Sent {
				sent[n.Type] = true
				continue
			}
			delete(s.notifs, k)
		}
	}
	for _, n := range ns {
		if sent[n.Type] {
			continue
		}
		if n.ID == "" {
			n.ID = domain.NewID()
		}
		n.UserID, n.ProductID = userID, productID
		s.notifs[n.ID] = n
	}
	return nil
}

// ListNotifications lists a user's reminders by schedule.
func (s *Store) ListNotifications(_ context.Context, userID string) ([]domain.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Notification{}
	for _, n := range s.notifs {
		if n.UserID == userID {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ScheduledAt.Before(out[j].ScheduledAt) })
	return out, nil
}

// DueNotifications returns unsent reminders scheduled at or before now.
func (s *Store) DueNotifications(_ context.Context, now time.Time, limit int) ([]domain.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Notification{}
	for _, n := range s.notifs {
		if !n.Sent && !n.ScheduledAt.After(now) {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ScheduledAt.Before(out[j].ScheduledAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MarkNotificationSent flags a reminder as delivered.
func (s *Store) MarkNotificationSent(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notifs[id]
	if !ok {
		return domain.ErrNotFound
	}
	n.Sent = true
	n.SentAt = &at
	s.notifs[id] = n
	return nil
}

// CreateExtractionLog appends a log entry.
func (s *Store) CreateExtractionLog(_ context.Context, l *domain.ExtractionLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.ID == "" {
		l.ID = domain.NewID()
	}
	l.CreatedAt = s.now().UTC()
	s.logs = append(s.logs, *l)
	return nil
}

// ListExtractionLogs returns newest logs first.
func (s *Store) ListExtractionLogs(_ context.Context, onlyFailures bool, limit int) ([]domain.ExtractionLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.ExtractionLog{}
	for i := len(s.logs) - 1; i >= 0; i-- {
		if onlyFailures && s.logs[i].Success {
			continue
		}
		out = append(out, s.logs[i])
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ListCategories returns categories sorted by slug ("other" last).
func (s *Store) ListCategories(context.Context) ([]domain.Category, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Category, 0, len(s.categories))
	for _, c := range s.categories {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slug == "other" || out[j].Slug == "other" {
			return out[j].Slug == "other" && out[i].Slug != "other"
		}
		return out[i].Slug < out[j].Slug
	})
	return out, nil
}

// UpsertCategory creates or renames a category.
func (s *Store) UpsertCategory(_ context.Context, c domain.Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.categories[c.Slug] = c
	return nil
}

// DeleteCategory removes a category.
func (s *Store) DeleteCategory(_ context.Context, slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.categories[slug]; !ok {
		return domain.ErrNotFound
	}
	delete(s.categories, slug)
	return nil
}
