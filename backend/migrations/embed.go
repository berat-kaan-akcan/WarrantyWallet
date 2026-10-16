// Package migrations embeds the ordered SQL migration files (NNNN_name.sql).
package migrations

import "embed"

// FS holds all *.sql migrations. Files are applied in lexical order exactly once.
//
//go:embed *.sql
var FS embed.FS
