// Package migrations embeds the SQL migration files for goose.
package migrations

import "embed"

// FS contains all goose SQL migrations.
//
//go:embed *.sql
var FS embed.FS
