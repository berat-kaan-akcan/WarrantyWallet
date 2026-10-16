//go:build pgx

package postgres

// Integration tests: run with a PostgreSQL database and the pgx driver:
//
//	TEST_DATABASE_URL=postgres://... go test -tags pgx ./internal/store/postgres/
//
// Each subtest gets a freshly migrated schema.

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store/storetest"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/migrations"
)

func TestContract(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	storetest.Run(t, func(t *testing.T) store.Store {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
			t.Fatal(err)
		}
		if _, err := Migrate(ctx, db, migrations.FS); err != nil {
			t.Fatal(err)
		}
		return New(db)
	})
}
