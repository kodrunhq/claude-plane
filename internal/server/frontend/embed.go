package frontend

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist/*
var FrontendFS embed.FS

// NewSPAHandler returns an http.Handler that serves the embedded frontend.
// For paths that match a static file (JS, CSS, images), it serves the file directly.
// For all other paths, it serves index.html to support client-side routing.
//
// Mount this handler LAST in the Chi router, after /api/v1/* and /ws/* routes,
// so API calls are not caught by the SPA fallback.
func NewSPAHandler() http.Handler {
	stripped, err := fs.Sub(FrontendFS, "dist")
	if err != nil {
		panic("frontend: failed to create sub filesystem: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(stripped))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the exact file
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Check if the file exists in the embedded FS
		if _, err := fs.Stat(stripped, path); err != nil {
			// File doesn't exist -- serve index.html for SPA routing
			r.URL.Path = "/"
		}

		fileServer.ServeHTTP(w, r)
	})
}
