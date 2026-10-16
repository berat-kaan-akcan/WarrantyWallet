//go:build pgx

package main

// Links the PostgreSQL driver. Build with: go build -tags pgx ./cmd/api
// (the Dockerfile does this). Without the tag the API runs on the in-memory store only.
import _ "github.com/jackc/pgx/v5/stdlib"
