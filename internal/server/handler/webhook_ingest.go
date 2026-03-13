package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/event"
)

// IngestHandler handles inbound webhook events from external sources.
// Each source is authenticated via a per-source HMAC secret (GitHub format).
// This handler is intentionally public — no JWT auth — relying on per-source secrets.
type IngestHandler struct {
	publisher event.Publisher
	secrets   map[string]string // source name -> HMAC secret (empty string = no auth)
	logger    *slog.Logger
}

// NewIngestHandler creates an IngestHandler with the given publisher and per-source secrets map.
func NewIngestHandler(publisher event.Publisher, secrets map[string]string, logger *slog.Logger) *IngestHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &IngestHandler{
		publisher: publisher,
		secrets:   secrets,
		logger:    logger,
	}
}

// RegisterIngestRoutes mounts the inbound webhook ingest route on the given router.
func RegisterIngestRoutes(r chi.Router, h *IngestHandler) {
	r.Post("/api/v1/webhooks/ingest/{source}", h.Ingest)
}

// Ingest handles POST /api/v1/webhooks/ingest/{source}.
//
// Steps:
//  1. Resolves the source from the URL param.
//  2. Returns 404 if the source is not configured.
//  3. Validates HMAC-SHA256 signature when the source has a non-empty secret.
//  4. Parses body as generic JSON.
//  5. Publishes a trigger.webhook event and returns 200 OK.
func (h *IngestHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	source := chi.URLParam(r, "source")

	secret, ok := h.secrets[source]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown source %q", source))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("webhook ingest: failed to read body", "source", source, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read request body")
		return
	}

	if secret != "" {
		if err := validateHMACSignature(body, secret, r.Header.Get("X-Hub-Signature-256")); err != nil {
			h.logger.Warn("webhook ingest: signature validation failed", "source", source, "error", err)
			writeError(w, http.StatusUnauthorized, "invalid signature")
			return
		}
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	evt := event.NewTriggerEvent(
		event.TypeTriggerWebhook,
		"webhook:"+source,
		payload,
	)

	if err := h.publisher.Publish(r.Context(), evt); err != nil {
		h.logger.Error("webhook ingest: failed to publish event", "source", source, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to publish event")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// validateHMACSignature verifies that the provided GitHub-format signature header
// (sha256=<hex>) matches the HMAC-SHA256 of body computed with secret.
// Returns a non-nil error if the signature is missing, malformed, or does not match.
func validateHMACSignature(body []byte, secret, sigHeader string) error {
	const prefix = "sha256="
	if sigHeader == "" {
		return fmt.Errorf("missing X-Hub-Signature-256 header")
	}
	if !strings.HasPrefix(sigHeader, prefix) {
		return fmt.Errorf("signature header must start with %q", prefix)
	}

	provided, err := hex.DecodeString(strings.TrimPrefix(sigHeader, prefix))
	if err != nil {
		return fmt.Errorf("signature header contains invalid hex: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)

	if !hmac.Equal(expected, provided) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}
