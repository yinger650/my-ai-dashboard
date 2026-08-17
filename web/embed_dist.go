package webui

import "embed"

// distFS holds the built frontend. `all:` includes files that start with '_'
// or '.', such as Vite's hashed asset chunks.
//
//go:embed all:dist
var distFS embed.FS
