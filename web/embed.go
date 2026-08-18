// Package webui embeds the built React frontend (web/dist) for board-server.
package webui

import (
	"io/fs"
)

// FS returns the embedded frontend filesystem rooted at the dist directory.
// It returns ok=false when only the placeholder is present (frontend not built
// with real assets), which callers may use to decide whether to serve the SPA.
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
