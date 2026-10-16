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

const userCols = `id, email, name, role, password_hash, created_at`

func scanUser(row interface{ Scan(...any) error }) (domain.User, error) {
	var u domain.User
	var role string
	err := row.Scan(&u.ID, &u.Email, &u.Name, &role, &u.PasswordHash, &u.CreatedAt)
	u.Role = domain.Role(role)
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
	currency, warranty_months, warranty_end, serial_number, notes, created_at, updated_at`

func scanProduct(row interface{ Scan(...any) error }) (domain.Product, error) {
	var p domain.Product
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Brand, &p.Model, &p.Category, &p.Seller, &p.PurchaseDate,
		&p.Price, &p.Currency, &p.WarrantyMonths, &p.WarrantyEnd, &p.SerialNumber, &p.Notes, &p.CreatedAt, &p.UpdatedAt)
	p.Currency = strings.TrimSpace(p.Currency)
	return p, err
}

// CreateProduct inserts a product.
func (s *Store) CreateProduct(ctx context.Context, p *domain.Product) error {
	if p.ID == "" {
		p.ID = domain.NewID()
	}
	return s.db.QueryRowContext(ctx, `INSERT INTO products (id, user_id, name, brand, model, category, seller,
		purchase_date, purchase_price, currency, warranty_months, warranty_end, serial_number, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING created_at, updated_at`,
		p.ID, p.UserID, p.Name, p.Brand, p.Model, p.Category, p.Seller, p.PurchaseDate, p.Price, p.Currency,
		p.WarrantyMonths, p.WarrantyEnd, p.SerialNumber, p.Notes).Scan(&p.CreatedAt, &p.UpdatedAt)
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
		q += fmt.Sprintf(" AND (name ILIKE $%[1]d OR brand ILIKE $%[1]d OR model ILIKE $%[1]d OR seller ILIKE $%[1]d)", len(args))
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
		notes=$14, updated_at=now() WHERE id=$1 AND user_id=$2 RETURNING created_at, updated_at`,
		p.ID, p.UserID, p.Name, p.Brand, p.Model, p.Category, p.Seller, p.PurchaseDate, p.Price, p.Currency,
		p.WarrantyMonths, p.WarrantyEnd, p.SerialNumber, p.Notes).Scan(&p.CreatedAt, &p.UpdatedAt)
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

const srCols = `id, user_id, product_id, problem, description, service_name, service_date, status, notes, under_warranty, created_at, updated_at`

func scanSR(row interface{ Scan(...any) error }) (domain.ServiceRecord, error) {
	var r domain.ServiceRecord
	var problem, status string
	err := row.Scan(&r.ID, &r.UserID, &r.ProductID, &problem, &r.Description, &r.ServiceName, &r.ServiceDate, &status, &r.Notes, &r.UnderWarranty, &r.CreatedAt, &r.UpdatedAt)
	r.Problem, r.Status = domain.ProblemType(problem), domain.ServiceStatus(status)
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
		service_name, service_date, status, notes, under_warranty)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10 WHERE EXISTS (SELECT 1 FROM products WHERE id=$3 AND user_id=$2)
		RETURNING created_at, updated_at`,
		r.ID, r.UserID, r.ProductID, string(r.Problem), r.Description, r.ServiceName, r.ServiceDate, string(r.Status),
		r.Notes, r.UnderWarranty).Scan(&r.CreatedAt, &r.UpdatedAt)
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
		service_date=$6, status=$7, notes=$8, under_warranty=$9, updated_at=now()
		WHERE id=$1 AND user_id=$2 RETURNING created_at, updated_at`,
		r.ID, r.UserID, string(r.Problem), r.Description, r.ServiceName, r.ServiceDate, string(r.Status), r.Notes,
		r.UnderWarranty).Scan(&r.CreatedAt, &r.UpdatedAt)
	return notFound(err)
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
	return s.db.QueryRowContext(ctx, `INSERT INTO extraction_logs (id, user_id, provider, success, error, raw_text)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING created_at`, l.ID, uid, l.Provider, l.Success, l.Error, l.RawText).Scan(&l.CreatedAt)
}

// ListExtractionLogs lists newest logs.
func (s *Store) ListExtractionLogs(ctx context.Context, onlyFailures bool, limit int) ([]domain.ExtractionLog, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT id, COALESCE(user_id::text,''), provider, success, error, raw_text, created_at FROM extraction_logs`
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
		if err := rows.Scan(&l.ID, &l.UserID, &l.Provider, &l.Success, &l.Error, &l.RawText, &l.CreatedAt); err != nil {
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
