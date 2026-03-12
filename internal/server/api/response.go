// Package api provides the REST API layer for the claude-plane server.
package api

import (
	"net/http"

	"github.com/kodrunhq/claude-plane/internal/server/httputil"
)

// writeJSON delegates to the shared httputil.WriteJSON helper.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	httputil.WriteJSON(w, status, data)
}

// writeError delegates to the shared httputil.WriteError helper.
func writeError(w http.ResponseWriter, status int, message string) {
	httputil.WriteError(w, status, message)
}
