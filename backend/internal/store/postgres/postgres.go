// Package postgres implements store.Store on PostgreSQL via database/sql.
//
// The package itself depends only on the standard library. The concrete driver
// (github.com/jackc/pgx/v5/stdlib, registered as "pgx") is linked in by the binary
// built with `-tags pgx` (see cmd/api/driver_pgx.go and backend/Dockerfile).
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store"
)

// Store is a PostgreSQL-backed store.Store.
type Store struct{ db *sql.DB }

var _ store.Store = (*Store)(nil)

// New wraps an open *sql.DB.
func New(db *sql.DB) *Store { return &Store{db: db} }

// Open opens a connection pool with the named driver and verifies connectivity.
func Open(ctx context.Context, driverName, dsn string) (*sql.DB, error) {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	var lastErr error
	for i := 0; i < 10; i++ { // DB container may still be starting
		if lastErr = db.PingContext(ctx); lastErr == nil {
			return db, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	_ = db.Close()
	return nil, fmt.Errorf("postgres not reachable: %w", lastErr)
}

// Migrate applies pending *.sql files from fsys in lexical order inside transactions.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) ([]string, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, err
	}
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var applied []string
	for _, name := range names {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, name).Scan(&exists); err != nil {
			return applied, err
		}
		if exists {
			continue
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return applied, err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return applied, err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback()
			return applied, err
		}
		if err := tx.Commit(); err != nil {
			return applied, err
		}
		applied = append(applied, name)
	}
	return applied, nil
}

// Ping checks connectivity.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key"))
}

func isFKViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "23503") || strings.Contains(err.Error(), "foreign key"))
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ---- users ----

const userCols = `id, email, name, role, password_hash, email_verified_at, created_at`

func scanUser(row interface{ Scan(...any) error }) (domain.User, error) {
	var u domain.User
	var role string
	var verified sql.NullTime
	err := row.Scan(&u.ID, &u.Email, &u.Name, &role, &u.PasswordHash, &verified, &u.CreatedAt)
	u.Role = domain.Role(role)
	if verified.Valid {
		t := verified.Time
		u.EmailVerifiedAt = &t
	}
	return u, err
}

// CreateUser inserts a user.
func (s *Store) CreateUser(ctx context.Context, u *domain.User) error {
	if u.ID == "" {
		u.ID = domain.NewID()
	}
	if u.Role == "" {
		u.Role = domain.RoleUser
	}
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO users (id, email, name, role, password_hash) VALUES ($1,$2,$3,$4,$5) RETURNING created_at`,
		u.ID, u.Email, u.Name, string(u.Role), u.PasswordHash).Scan(&u.CreatedAt)
	if isUniqueViolation(err) {
		return domain.ErrConflict
	}
	return err
}

// GetUserByEmail finds a user by email (citext → case-insensitive).
func (s *Store) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email=$1`, strings.TrimSpace(email)))
	return u, notFound(err)
}

// GetUserByID finds a user by id.
func (s *Store) GetUserByID(ctx context.Context, id string) (domain.User, error) {
	if !validUUID(id) {
		return domain.User{}, domain.ErrNotFound
	}
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id=$1`, id))
	return u, notFound(err)
}

// ListUsers lists all users.
func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetUserRole updates a user's role.
func (s *Store) SetUserRole(ctx context.Context, id string, role domain.Role) error {
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return affected(s.db.ExecContext(ctx, `UPDATE users SET role=$2 WHERE id=$1`, id, string(role)))
}

// MarkEmailVerified records the first successful verification.
func (s *Store) MarkEmailVerified(ctx context.Context, id string, at time.Time) error {
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return affected(s.db.ExecContext(ctx, `UPDATE users SET email_verified_at=COALESCE(email_verified_at, $2) WHERE id=$1`, id, at))
}

// DeleteUser deletes a user (cascades).
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return affected(s.db.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, id))
}

// UpdateUserProfile changes the display name.
func (s *Store) UpdateUserProfile(ctx context.Context, id, name string) error {
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return affected(s.db.ExecContext(ctx, `UPDATE users SET name=$2 WHERE id=$1`, id, name))
}

// SetUserPassword replaces the password hash.
func (s *Store) SetUserPassword(ctx context.Context, id, hash string) error {
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return affected(s.db.ExecContext(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, id, hash))
}

// ---- auth tokens ----

// CreateAuthToken stores a hashed token.
func (s *Store) CreateAuthToken(ctx context.Context, t *domain.AuthToken) error {
	if !validUUID(t.UserID) {
		return domain.ErrNotFound
	}
	if t.ID == "" {
		t.ID = domain.NewID()
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO auth_tokens (id, user_id, kind, token_hash, expires_at) VALUES ($1,$2,$3,$4,$5) RETURNING created_at`,
		t.ID, t.UserID, string(t.Kind), t.TokenHash, t.ExpiresAt).Scan(&t.CreatedAt)
	switch {
	case isUniqueViolation(err):
		return domain.ErrConflict
	case isFKViolation(err):
		return domain.ErrNotFound
	}
	return err
}

// ConsumeAuthToken atomically marks a valid token as used.
func (s *Store) ConsumeAuthToken(ctx context.Context, kind domain.AuthTokenKind, hash string, now time.Time) (domain.AuthToken, error) {
	var t domain.AuthToken
	var k string
	var used time.Time
	err := s.db.QueryRowContext(ctx, `UPDATE auth_tokens SET used_at=$3
		WHERE kind=$1 AND token_hash=$2 AND used_at IS NULL AND expires_at > $3
		RETURNING id, user_id, kind, token_hash, expires_at, used_at, created_at`, string(kind), hash, now).
		Scan(&t.ID, &t.UserID, &k, &t.TokenHash, &t.ExpiresAt, &used, &t.CreatedAt)
	if err != nil {
		return domain.AuthToken{}, notFound(err)
	}
	t.Kind, t.UsedAt = domain.AuthTokenKind(k), &used
	return t, nil
}

// RevokeAuthTokens marks all unused tokens of a kind as used.
func (s *Store) RevokeAuthTokens(ctx context.Context, userID string, kind domain.AuthTokenKind, now time.Time) error {
	if !validUUID(userID) {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE auth_tokens SET used_at=$3 WHERE user_id=$1 AND kind=$2 AND used_at IS NULL`,
		userID, string(kind), now)
	return err
}

// ---- products ----

const productCols = `id, user_id, name, brand, model, category, seller, purchase_date, purchase_price::float8,
	currency, warranty_months, warranty_end, serial_number, barcode, notes, created_at, updated_at`

func scanProduct(row interface{ Scan(...any) error }) (domain.Product, error) {
	var p domain.Product
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Brand, &p.Model, &p.Category, &p.Seller, &p.PurchaseDate,
		&p.Price, &p.Currency, &p.WarrantyMonths, &p.WarrantyEnd, &p.SerialNumber, &p.Barcode, &p.Notes, &p.CreatedAt, &p.UpdatedAt)
	p.Currency = strings.TrimSpace(p.Currency)
	return p, err
}

// CreateProduct inserts a product.
func (s *Store) CreateProduct(ctx context.Context, p *domain.Product) error {
	if p.ID == "" {
		p.ID = domain.NewID()
	}
	return s.db.QueryRowContext(ctx, `INSERT INTO products (id, user_id, name, brand, model, category, seller,
		purchase_date, purchase_price, currency, warranty_months, warranty_end, serial_number, notes, barcode)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING created_at, updated_at`,
		p.ID, p.UserID, p.Name, p.Brand, p.Model, p.Category, p.Seller, p.PurchaseDate, p.Price, p.Currency,
		p.WarrantyMonths, p.WarrantyEnd, p.SerialNumber, p.Notes, p.Barcode).Scan(&p.CreatedAt, &p.UpdatedAt)
}

// GetProduct returns an owned product.
func (s *Store) GetProduct(ctx context.Context, userID, id string) (domain.Product, error) {
	if !validUUID(id) {
		return domain.Product{}, domain.ErrNotFound
	}
	p, err := scanProduct(s.db.QueryRowContext(ctx, `SELECT `+productCols+` FROM products WHERE id=$1 AND user_id=$2`, id, userID))
	return p, notFound(err)
}

// ListProducts lists owned products with filters.
func (s *Store) ListProducts(ctx context.Context, userID string, f store.ProductFilter) ([]domain.Product, error) {
	q := `SELECT ` + productCols + ` FROM products WHERE user_id=$1`
	args := []any{userID}
	if f.Category != "" {
		args = append(args, f.Category)
		q += fmt.Sprintf(" AND category=$%d", len(args))
	}
	if s := strings.TrimSpace(f.Query); s != "" {
		args = append(args, "%"+escapeLike(s)+"%")
		q += fmt.Sprintf(" AND (name ILIKE $%[1]d OR brand ILIKE $%[1]d OR model ILIKE $%[1]d OR seller ILIKE $%[1]d"+
			" OR serial_number ILIKE $%[1]d OR barcode ILIKE $%[1]d)", len(args))
	}
	if c := strings.TrimSpace(f.Code); c != "" {
		args = append(args, strings.ToLower(c))
		q += fmt.Sprintf(" AND (lower(barcode)=$%[1]d OR lower(serial_number)=$%[1]d)", len(args))
	}
	if !f.EndFrom.IsZero() {
		args = append(args, f.EndFrom)
		q += fmt.Sprintf(" AND warranty_end >= $%d", len(args))
	}
	if !f.EndTo.IsZero() {
		args = append(args, f.EndTo)
		q += fmt.Sprintf(" AND warranty_end <= $%d", len(args))
	}
	q += " ORDER BY created_at DESC, id DESC"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateProduct updates an owned product.
func (s *Store) UpdateProduct(ctx context.Context, p *domain.Product) error {
	if !validUUID(p.ID) {
		return domain.ErrNotFound
	}
	err := s.db.QueryRowContext(ctx, `UPDATE products SET name=$3, brand=$4, model=$5, category=$6, seller=$7,
		purchase_date=$8, purchase_price=$9, currency=$10, warranty_months=$11, warranty_end=$12, serial_number=$13,
		notes=$14, barcode=$15, updated_at=now() WHERE id=$1 AND user_id=$2 RETURNING created_at, updated_at`,
		p.ID, p.UserID, p.Name, p.Brand, p.Model, p.Category, p.Seller, p.PurchaseDate, p.Price, p.Currency,
		p.WarrantyMonths, p.WarrantyEnd, p.SerialNumber, p.Notes, p.Barcode).Scan(&p.CreatedAt, &p.UpdatedAt)
	return notFound(err)
}

// DeleteProduct deletes an owned product (cascades).
func (s *Store) DeleteProduct(ctx context.Context, userID, id string) error {
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return affected(s.db.ExecContext(ctx, `DELETE FROM products WHERE id=$1 AND user_id=$2`, id, userID))
}

// ---- documents ----

const docCols = `id, user_id, product_id, COALESCE(service_record_id::text, ''), type, file_key, content_type, size_bytes, original_name, created_at`

func scanDoc(row interface{ Scan(...any) error }) (domain.Document, error) {
	var d domain.Document
	var typ string
	err := row.Scan(&d.ID, &d.UserID, &d.ProductID, &d.ServiceRecordID, &typ, &d.FileKey, &d.ContentType, &d.SizeBytes, &d.OriginalName, &d.CreatedAt)
	d.Type = domain.DocumentType(typ)
	return d, err
}

// CreateDocument inserts metadata if the product is owned by the user.
func (s *Store) CreateDocument(ctx context.Context, d *domain.Document) error {
	if !validUUID(d.ProductID) {
		return domain.ErrNotFound
	}
	if d.ID == "" {
		d.ID = domain.NewID()
	}
	var sr any
	if d.ServiceRecordID != "" {
		sr = d.ServiceRecordID
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO documents (id, user_id, product_id, service_record_id, type, file_key,
		content_type, size_bytes, original_name)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9 WHERE EXISTS (SELECT 1 FROM products WHERE id=$3 AND user_id=$2)
		RETURNING created_at`,
		d.ID, d.UserID, d.ProductID, sr, string(d.Type), d.FileKey, d.ContentType, d.SizeBytes, d.OriginalName).Scan(&d.CreatedAt)
	if isFKViolation(err) {
		return domain.ErrNotFound
	}
	return notFound(err)
}

// GetDocument returns an owned document.
func (s *Store) GetDocument(ctx context.Context, userID, id string) (domain.Document, error) {
	if !validUUID(id) {
		return domain.Document{}, domain.ErrNotFound
	}
	d, err := scanDoc(s.db.QueryRowContext(ctx, `SELECT `+docCols+` FROM documents WHERE id=$1 AND user_id=$2`, id, userID))
	return d, notFound(err)
}

// ListDocuments lists documents of a product.
func (s *Store) ListDocuments(ctx context.Context, userID, productID string) ([]domain.Document, error) {
	if !validUUID(productID) {
		return []domain.Document{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+docCols+` FROM documents WHERE user_id=$1 AND product_id=$2 ORDER BY created_at, id`, userID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Document{}
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteDocument deletes an owned document.
func (s *Store) DeleteDocument(ctx context.Context, userID, id string) error {
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return affected(s.db.ExecContext(ctx, `DELETE FROM documents WHERE id=$1 AND user_id=$2`, id, userID))
}

// ---- service records ----

const srCols = `id, user_id, product_id, problem, description, service_name, service_date, status, notes, under_warranty, cost::float8, resolved_at, created_at, updated_at`

func scanSR(row interface{ Scan(...any) error }) (domain.ServiceRecord, error) {
	var r domain.ServiceRecord
	var problem, status string
	var resolved sql.NullTime
	err := row.Scan(&r.ID, &r.UserID, &r.ProductID, &problem, &r.Description, &r.ServiceName, &r.ServiceDate, &status, &r.Notes, &r.UnderWarranty, &r.Cost, &resolved, &r.CreatedAt, &r.UpdatedAt)
	r.Problem, r.Status = domain.ProblemType(problem), domain.ServiceStatus(status)
	if resolved.Valid {
		t := resolved.Time
		r.ResolvedAt = &t
	}
	return r, err
}

// CreateServiceRecord inserts a record for an owned product.
func (s *Store) CreateServiceRecord(ctx context.Context, r *domain.ServiceRecord) error {
	if !validUUID(r.ProductID) {
		return domain.ErrNotFound
	}
	if r.ID == "" {
		r.ID = domain.NewID()
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO service_records (id, user_id, product_id, problem, description,
		service_name, service_date, status, notes, under_warranty, cost, resolved_at)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12 WHERE EXISTS (SELECT 1 FROM products WHERE id=$3 AND user_id=$2)
		RETURNING created_at, updated_at`,
		r.ID, r.UserID, r.ProductID, string(r.Problem), r.Description, r.ServiceName, r.ServiceDate, string(r.Status),
		r.Notes, r.UnderWarranty, r.Cost, r.ResolvedAt).Scan(&r.CreatedAt, &r.UpdatedAt)
	return notFound(err)
}

// GetServiceRecord returns an owned record.
func (s *Store) GetServiceRecord(ctx context.Context, userID, id string) (domain.ServiceRecord, error) {
	if !validUUID(id) {
		return domain.ServiceRecord{}, domain.ErrNotFound
	}
	r, err := scanSR(s.db.QueryRowContext(ctx, `SELECT `+srCols+` FROM service_records WHERE id=$1 AND user_id=$2`, id, userID))
	return r, notFound(err)
}

// ListServiceRecords lists records (optionally for one product).
func (s *Store) ListServiceRecords(ctx context.Context, userID, productID string) ([]domain.ServiceRecord, error) {
	q := `SELECT ` + srCols + ` FROM service_records WHERE user_id=$1`
	args := []any{userID}
	if productID != "" {
		if !validUUID(productID) {
			return []domain.ServiceRecord{}, nil
		}
		q += ` AND product_id=$2`
		args = append(args, productID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY created_at DESC, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ServiceRecord{}
	for rows.Next() {
		r, err := scanSR(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateServiceRecord updates an owned record.
func (s *Store) UpdateServiceRecord(ctx context.Context, r *domain.ServiceRecord) error {
	if !validUUID(r.ID) {
		return domain.ErrNotFound
	}
	err := s.db.QueryRowContext(ctx, `UPDATE service_records SET problem=$3, description=$4, service_name=$5,
		service_date=$6, status=$7, notes=$8, under_warranty=$9, cost=$10, resolved_at=$11, updated_at=now()
		WHERE id=$1 AND user_id=$2 RETURNING created_at, updated_at`,
		r.ID, r.UserID, string(r.Problem), r.Description, r.ServiceName, r.ServiceDate, string(r.Status), r.Notes,
		r.UnderWarranty, r.Cost, r.ResolvedAt).Scan(&r.CreatedAt, &r.UpdatedAt)
	return notFound(err)
}

// DeleteServiceRecord removes an owned record; its events cascade and documents are unlinked (FK SET NULL).
func (s *Store) DeleteServiceRecord(ctx context.Context, userID, id string) error {
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM service_records WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// AddServiceEvent appends to an owned record's timeline.
func (s *Store) AddServiceEvent(ctx context.Context, userID string, e *domain.ServiceEvent) error {
	if !validUUID(e.ServiceRecordID) {
		return domain.ErrNotFound
	}
	if e.ID == "" {
		e.ID = domain.NewID()
	}
	var at any // keep a given timestamp (backup import), else now()
	if !e.CreatedAt.IsZero() {
		at = e.CreatedAt
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO service_events (id, service_record_id, status, note, created_at)
		SELECT $1,$2,$3,$4,COALESCE($6::timestamptz, now()) WHERE EXISTS (SELECT 1 FROM service_records WHERE id=$2 AND user_id=$5)
		RETURNING created_at`, e.ID, e.ServiceRecordID, string(e.Status), e.Note, userID, at).Scan(&e.CreatedAt)
	return notFound(err)
}

// ListServiceEvents returns an owned record's timeline, oldest first.
func (s *Store) ListServiceEvents(ctx context.Context, userID, recordID string) ([]domain.ServiceEvent, error) {
	if _, err := s.GetServiceRecord(ctx, userID, recordID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, service_record_id, status, note, created_at FROM service_events
		WHERE service_record_id=$1 ORDER BY created_at, id`, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ServiceEvent{}
	for rows.Next() {
		var e domain.ServiceEvent
		var status string
		if err := rows.Scan(&e.ID, &e.ServiceRecordID, &status, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Status = domain.ServiceStatus(status)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- notifications ----

const notifCols = `id, user_id, product_id, type, message, scheduled_at, sent, sent_at`

func scanNotif(row interface{ Scan(...any) error }) (domain.Notification, error) {
	var n domain.Notification
	var typ string
	var sentAt sql.NullTime
	err := row.Scan(&n.ID, &n.UserID, &n.ProductID, &typ, &n.Message, &n.ScheduledAt, &n.Sent, &sentAt)
	n.Type = domain.NotificationType(typ)
	if sentAt.Valid {
		t := sentAt.Time
		n.SentAt = &t
	}
	return n, err
}

// ReplaceNotifications replaces pending reminders of a product; already-sent ones are kept.
func (s *Store) ReplaceNotifications(ctx context.Context, userID, productID string, ns []domain.Notification) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM notifications WHERE product_id=$1 AND user_id=$2 AND NOT sent`, productID, userID); err != nil {
		return err
	}
	for _, n := range ns {
		if n.ID == "" {
			n.ID = domain.NewID()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (id, user_id, product_id, type, message, scheduled_at)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (product_id, type) DO NOTHING`,
			n.ID, userID, productID, string(n.Type), n.Message, n.ScheduledAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListNotifications lists a user's reminders.
func (s *Store) ListNotifications(ctx context.Context, userID string) ([]domain.Notification, error) {
	return s.queryNotifs(ctx, `SELECT `+notifCols+` FROM notifications WHERE user_id=$1 ORDER BY scheduled_at`, userID)
}

// DueNotifications returns unsent reminders that are due.
func (s *Store) DueNotifications(ctx context.Context, now time.Time, limit int) ([]domain.Notification, error) {
	if limit <= 0 {
		limit = 100
	}
	return s.queryNotifs(ctx, `SELECT `+notifCols+` FROM notifications WHERE NOT sent AND scheduled_at <= $1
		ORDER BY scheduled_at LIMIT $2`, now, limit)
}

func (s *Store) queryNotifs(ctx context.Context, q string, args ...any) ([]domain.Notification, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Notification{}
	for rows.Next() {
		n, err := scanNotif(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// MarkNotificationSent flags a reminder as sent.
func (s *Store) MarkNotificationSent(ctx context.Context, id string, at time.Time) error {
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return affected(s.db.ExecContext(ctx, `UPDATE notifications SET sent=true, sent_at=$2 WHERE id=$1`, id, at))
}

// ---- extraction logs ----

// CreateExtractionLog inserts a log row.
func (s *Store) CreateExtractionLog(ctx context.Context, l *domain.ExtractionLog) error {
	if l.ID == "" {
		l.ID = domain.NewID()
	}
	var uid any
	if l.UserID != "" {
		uid = l.UserID
	}
	if l.Kind == "" {
		l.Kind = domain.LogReceipt
	}
	return s.db.QueryRowContext(ctx, `INSERT INTO extraction_logs (id, user_id, kind, provider, model, success, error, raw_text)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at`, l.ID, uid, l.Kind, l.Provider, l.Model, l.Success, l.Error, l.RawText).Scan(&l.CreatedAt)
}

// ListExtractionLogs lists newest logs.
func (s *Store) ListExtractionLogs(ctx context.Context, onlyFailures bool, limit int) ([]domain.ExtractionLog, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT id, COALESCE(user_id::text,''), kind, provider, model, success, error, raw_text, created_at FROM extraction_logs`
	if onlyFailures {
		q += ` WHERE NOT success`
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY created_at DESC, id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ExtractionLog{}
	for rows.Next() {
		var l domain.ExtractionLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Kind, &l.Provider, &l.Model, &l.Success, &l.Error, &l.RawText, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ---- categories ----

// ListCategories lists categories ("other" last).
func (s *Store) ListCategories(ctx context.Context) ([]domain.Category, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT slug, name_tr FROM categories ORDER BY (slug='other'), slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Category{}
	for rows.Next() {
		var c domain.Category
		if err := rows.Scan(&c.Slug, &c.NameTR); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpsertCategory creates or renames a category.
func (s *Store) UpsertCategory(ctx context.Context, c domain.Category) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO categories (slug, name_tr) VALUES ($1,$2)
		ON CONFLICT (slug) DO UPDATE SET name_tr=EXCLUDED.name_tr`, c.Slug, c.NameTR)
	return err
}

// DeleteCategory deletes a category.
func (s *Store) DeleteCategory(ctx context.Context, slug string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM categories WHERE slug=$1`, slug))
}

// ---- helpers ----

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// validUUID prevents "invalid input syntax for type uuid" errors from surfacing as 500s.
func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
				return false
			}
		}
	}
	return true
}

// ---- device tokens ----

// SaveDeviceToken upserts a push token; the token moves to the user registering it.
func (s *Store) SaveDeviceToken(ctx context.Context, d *domain.DeviceToken) error {
	if !validUUID(d.UserID) {
		return domain.ErrNotFound
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO device_tokens (token, user_id, platform) VALUES ($1,$2,$3)
		ON CONFLICT (token) DO UPDATE SET user_id=EXCLUDED.user_id, platform=EXCLUDED.platform, last_seen_at=now()
		RETURNING created_at, last_seen_at`, d.Token, d.UserID, d.Platform).Scan(&d.CreatedAt, &d.LastSeenAt)
	if isFKViolation(err) {
		return domain.ErrNotFound
	}
	return err
}

// DeleteDeviceToken removes a token; userID "" ignores the owner.
func (s *Store) DeleteDeviceToken(ctx context.Context, userID, token string) error {
	if userID == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM device_tokens WHERE token=$1`, token)
		return err
	}
	if !validUUID(userID) {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM device_tokens WHERE token=$1 AND user_id=$2`, token, userID)
	return err
}

// ListDeviceTokens returns a user's tokens, most recently seen first.
func (s *Store) ListDeviceTokens(ctx context.Context, userID string) ([]domain.DeviceToken, error) {
	if !validUUID(userID) {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT token, user_id, platform, created_at, last_seen_at FROM device_tokens
		WHERE user_id=$1 ORDER BY last_seen_at DESC, token`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DeviceToken
	for rows.Next() {
		var d domain.DeviceToken
		if err := rows.Scan(&d.Token, &d.UserID, &d.Platform, &d.CreatedAt, &d.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---- families ----

// CreateFamily creates a family and its owner membership in one transaction.
func (s *Store) CreateFamily(ctx context.Context, f *domain.Family) error {
	if !validUUID(f.OwnerID) {
		return domain.ErrNotFound
	}
	if f.ID == "" {
		f.ID = domain.NewID()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	err = tx.QueryRowContext(ctx, `INSERT INTO families (id, name, owner_id) VALUES ($1,$2,$3) RETURNING created_at`,
		f.ID, f.Name, f.OwnerID).Scan(&f.CreatedAt)
	if isFKViolation(err) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO family_members (user_id, family_id, role, joined_at) VALUES ($1,$2,'owner',$3)`,
		f.OwnerID, f.ID, f.CreatedAt)
	if isUniqueViolation(err) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// GetFamilyForUser returns the family the user belongs to.
func (s *Store) GetFamilyForUser(ctx context.Context, userID string) (domain.Family, error) {
	var f domain.Family
	if !validUUID(userID) {
		return f, domain.ErrNotFound
	}
	err := s.db.QueryRowContext(ctx, `SELECT f.id, f.name, f.owner_id, f.created_at FROM families f
		JOIN family_members m ON m.family_id=f.id WHERE m.user_id=$1`, userID).Scan(&f.ID, &f.Name, &f.OwnerID, &f.CreatedAt)
	return f, notFound(err)
}

// ListFamilyMembers lists members with name and e-mail, owner first.
func (s *Store) ListFamilyMembers(ctx context.Context, familyID string) ([]domain.FamilyMemberInfo, error) {
	out := []domain.FamilyMemberInfo{}
	if !validUUID(familyID) {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.name, u.email, m.role, m.joined_at FROM family_members m
		JOIN users u ON u.id=m.user_id WHERE m.family_id=$1
		ORDER BY (m.role='owner') DESC, m.joined_at, u.id`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m domain.FamilyMemberInfo
		var role string
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &role, &m.JoinedAt); err != nil {
			return nil, err
		}
		m.Role = domain.FamilyRole(role)
		out = append(out, m)
	}
	return out, rows.Err()
}

// RenameFamily changes the family name.
func (s *Store) RenameFamily(ctx context.Context, familyID, name string) error {
	if !validUUID(familyID) {
		return domain.ErrNotFound
	}
	return affected(s.db.ExecContext(ctx, `UPDATE families SET name=$2 WHERE id=$1`, familyID, name))
}

// AddFamilyMember adds a member; the primary key on user_id enforces one family per user.
func (s *Store) AddFamilyMember(ctx context.Context, familyID, userID string) error {
	if !validUUID(familyID) || !validUUID(userID) {
		return domain.ErrNotFound
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO family_members (user_id, family_id, role) VALUES ($1,$2,'member')`, userID, familyID)
	switch {
	case isUniqueViolation(err):
		return domain.ErrConflict
	case isFKViolation(err):
		return domain.ErrNotFound
	}
	return err
}

// RemoveFamilyMember removes a member.
func (s *Store) RemoveFamilyMember(ctx context.Context, familyID, userID string) error {
	if !validUUID(familyID) || !validUUID(userID) {
		return domain.ErrNotFound
	}
	return affected(s.db.ExecContext(ctx, `DELETE FROM family_members WHERE family_id=$1 AND user_id=$2`, familyID, userID))
}

// DeleteFamily removes the family (memberships cascade).
func (s *Store) DeleteFamily(ctx context.Context, familyID string) error {
	if !validUUID(familyID) {
		return domain.ErrNotFound
	}
	return affected(s.db.ExecContext(ctx, `DELETE FROM families WHERE id=$1`, familyID))
}
