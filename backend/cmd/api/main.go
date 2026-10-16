// Command api runs the Warranty Wallet REST API.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/auth"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/config"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/extract"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/httpapi"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/notify"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/storage"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store/memory"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store/postgres"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/migrations"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(nil)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var st store.Store
	if cfg.DatabaseURL != "" {
		if !slices.Contains(sql.Drivers(), "pgx") {
			return errors.New("DATABASE_URL is set but the binary was built without the pgx driver (build with -tags pgx)")
		}
		db, err := postgres.Open(ctx, "pgx", cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer db.Close()
		applied, err := postgres.Migrate(ctx, db, migrations.FS)
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		logger.Info("database ready", "migrations_applied", applied)
		st = postgres.New(db)
	} else {
		logger.Warn("DATABASE_URL not set: using IN-MEMORY store (data is lost on restart)")
		st = memory.New()
	}

	files, err := storage.NewLocal(cfg.StorageDir)
	if err != nil {
		return err
	}

	gem := &extract.GeminiExtractor{
		APIKey:     cfg.GeminiAPIKey,
		Model:      cfg.GeminiModel,
		Fallbacks:  cfg.GeminiFallbacks,
		BaseURL:    cfg.GeminiBaseURL,
		HTTPClient: &http.Client{Timeout: cfg.GeminiTimeout},
	}
	extractor := &extract.Fallback{Primary: gem, Secondary: extract.RuleExtractor{}, Logger: logger}
	if cfg.GeminiAPIKey == "" {
		logger.Warn("GEMINI_API_KEY not set: receipt extraction uses the rule-based parser only")
	} else {
		logger.Info("gemini extraction enabled", "model", cfg.GeminiModel, "fallbacks", cfg.GeminiFallbacks)
	}

	srv := &httpapi.Server{
		Store: st, Files: files, Extractor: extractor,
		Tokens:      auth.Tokens{Secret: []byte(cfg.JWTSecret), TTL: cfg.TokenTTL},
		Hasher:      auth.Hasher{},
		Logger:      logger,
		MaxUpload:   cfg.MaxUploadMB << 20,
		AdminEmails: cfg.AdminEmails,
		CORSOrigins: cfg.CORSOrigins,
		Version:     version,
	}

	worker := &notify.Worker{Store: st, Interval: cfg.NotifyEvery, Logger: logger}
	go worker.Run(ctx)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.HTTPAddr, "version", version, "store", strings.TrimPrefix(fmt.Sprintf("%T", st), "*"))
		errCh <- httpServer.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
	return nil
}
