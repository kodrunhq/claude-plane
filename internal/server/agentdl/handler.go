package agentdl

import (
	"io"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// validPlatforms is the set of supported platform identifiers.
var validPlatforms = map[string]bool{
	"linux-amd64":  true,
	"linux-arm64":  true,
	"darwin-amd64": true,
	"darwin-arm64": true,
}

// Handler serves pre-built agent binaries from an embedded filesystem.
type Handler struct {
	fs fs.FS
}

// NewHandler returns a Handler that reads binaries from the given fs.FS.
// Pass AgentBinariesFS for production use; use fstest.MapFS in tests.
func NewHandler(binariesFS fs.FS) *Handler {
	return &Handler{fs: binariesFS}
}

// ServeDownload handles GET /dl/agent/{platform}.
// platform format: "{os}-{arch}", e.g. "linux-amd64", "darwin-arm64".
func (h *Handler) ServeDownload(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")

	if !validPlatforms[platform] {
		http.Error(w, "invalid platform; valid: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64", http.StatusBadRequest)
		return
	}

	filename := "binaries/claude-plane-agent-" + platform
	f, err := h.fs.Open(filename)
	if err != nil {
		http.Error(w, "agent binary not available for "+platform, http.StatusNotFound)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=claude-plane-agent")
	if _, err := io.Copy(w, f); err != nil {
		// Connection was likely closed by the client; nothing actionable here.
		return
	}
}

// RegisterRoutes mounts the download routes on the given chi router.
func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/dl/agent/{platform}", h.ServeDownload)
}
