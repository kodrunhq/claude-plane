package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// TriggerCRUDStore is the persistence interface required by TriggerHandler.
type TriggerCRUDStore interface {
	CreateJobTrigger(ctx context.Context, t store.JobTrigger) (*store.JobTrigger, error)
	ListJobTriggers(ctx context.Context, jobID string) ([]store.JobTrigger, error)
	UpdateJobTrigger(ctx context.Context, triggerID, eventType, filter string) (*store.JobTrigger, error)
	ToggleJobTrigger(ctx context.Context, triggerID string) (*store.JobTrigger, error)
	DeleteJobTrigger(ctx context.Context, triggerID string) error
}

// TriggerHandler handles REST endpoints for job trigger CRUD.
type TriggerHandler struct {
	store TriggerCRUDStore
}

// NewTriggerHandler creates a new TriggerHandler.
func NewTriggerHandler(store TriggerCRUDStore) *TriggerHandler {
	return &TriggerHandler{store: store}
}

// RegisterTriggerRoutes mounts all trigger-related routes on the given router.
func RegisterTriggerRoutes(r chi.Router, h *TriggerHandler) {
	r.Get("/api/v1/jobs/{jobID}/triggers", h.ListTriggers)
	r.Post("/api/v1/jobs/{jobID}/triggers", h.CreateTrigger)
	r.Put("/api/v1/triggers/{triggerID}", h.UpdateTrigger)
	r.Post("/api/v1/triggers/{triggerID}/toggle", h.ToggleTrigger)
	r.Delete("/api/v1/triggers/{triggerID}", h.DeleteTrigger)
}

// createTriggerRequest is the JSON body for POST /api/v1/jobs/{jobID}/triggers.
type createTriggerRequest struct {
	EventType string `json:"event_type"`
	Filter    string `json:"filter"`
}

// ListTriggers handles GET /api/v1/jobs/{jobID}/triggers.
func (h *TriggerHandler) ListTriggers(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	triggers, err := h.store.ListJobTriggers(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if triggers == nil {
		triggers = []store.JobTrigger{}
	}
	writeJSON(w, http.StatusOK, triggers)
}

// CreateTrigger handles POST /api/v1/jobs/{jobID}/triggers.
func (h *TriggerHandler) CreateTrigger(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	var req createTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.EventType == "" {
		writeError(w, http.StatusBadRequest, "event_type is required")
		return
	}

	trigger := store.JobTrigger{
		JobID:     jobID,
		EventType: req.EventType,
		Filter:    req.Filter,
		Enabled:   true,
	}

	created, err := h.store.CreateJobTrigger(r.Context(), trigger)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// updateTriggerRequest is the JSON body for PUT /api/v1/triggers/{triggerID}.
type updateTriggerRequest struct {
	EventType string `json:"event_type"`
	Filter    string `json:"filter"`
}

// UpdateTrigger handles PUT /api/v1/triggers/{triggerID}.
func (h *TriggerHandler) UpdateTrigger(w http.ResponseWriter, r *http.Request) {
	triggerID := chi.URLParam(r, "triggerID")

	var req updateTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.EventType == "" {
		writeError(w, http.StatusBadRequest, "event_type is required")
		return
	}

	updated, err := h.store.UpdateJobTrigger(r.Context(), triggerID, req.EventType, req.Filter)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "trigger not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// ToggleTrigger handles POST /api/v1/triggers/{triggerID}/toggle.
func (h *TriggerHandler) ToggleTrigger(w http.ResponseWriter, r *http.Request) {
	triggerID := chi.URLParam(r, "triggerID")

	toggled, err := h.store.ToggleJobTrigger(r.Context(), triggerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "trigger not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toggled)
}

// DeleteTrigger handles DELETE /api/v1/triggers/{triggerID}.
func (h *TriggerHandler) DeleteTrigger(w http.ResponseWriter, r *http.Request) {
	triggerID := chi.URLParam(r, "triggerID")

	if err := h.store.DeleteJobTrigger(r.Context(), triggerID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "trigger not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
