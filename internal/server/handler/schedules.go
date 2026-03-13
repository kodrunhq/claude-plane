package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/robfig/cron/v3"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// ScheduleCRUDStore is the persistence interface required by ScheduleHandler.
type ScheduleCRUDStore interface {
	CreateSchedule(ctx context.Context, p store.CreateScheduleParams) (*store.CronSchedule, error)
	GetSchedule(ctx context.Context, scheduleID string) (*store.CronSchedule, error)
	ListSchedulesByJob(ctx context.Context, jobID string) ([]store.CronSchedule, error)
	UpdateSchedule(ctx context.Context, p store.UpdateScheduleParams) (*store.CronSchedule, error)
	SetScheduleEnabled(ctx context.Context, scheduleID string, enabled bool) error
	DeleteSchedule(ctx context.Context, scheduleID string) error
}

// ScheduleReloader is the interface for hot-reloading cron entries after CRUD changes.
type ScheduleReloader interface {
	ReloadSchedule(ctx context.Context, scheduleID string) error
	RemoveSchedule(scheduleID string)
}

// ScheduleHandler handles REST endpoints for cron schedule CRUD.
type ScheduleHandler struct {
	store     ScheduleCRUDStore
	jobStore  store.JobStoreIface
	scheduler ScheduleReloader
	getClaims ClaimsGetter
}

// NewScheduleHandler creates a new ScheduleHandler.
func NewScheduleHandler(
	store ScheduleCRUDStore,
	jobStore store.JobStoreIface,
	scheduler ScheduleReloader,
	getClaims ClaimsGetter,
) *ScheduleHandler {
	return &ScheduleHandler{
		store:     store,
		jobStore:  jobStore,
		scheduler: scheduler,
		getClaims: getClaims,
	}
}

// RegisterScheduleRoutes mounts all schedule-related routes on the given router.
func RegisterScheduleRoutes(r chi.Router, h *ScheduleHandler) {
	r.Get("/api/v1/jobs/{jobID}/schedules", h.ListSchedules)
	r.Post("/api/v1/jobs/{jobID}/schedules", h.CreateSchedule)
	r.Get("/api/v1/schedules/{scheduleID}", h.GetSchedule)
	r.Put("/api/v1/schedules/{scheduleID}", h.UpdateSchedule)
	r.Delete("/api/v1/schedules/{scheduleID}", h.DeleteSchedule)
	r.Post("/api/v1/schedules/{scheduleID}/pause", h.PauseSchedule)
	r.Post("/api/v1/schedules/{scheduleID}/resume", h.ResumeSchedule)
}

// createScheduleRequest is the JSON body for POST /api/v1/jobs/{jobID}/schedules.
type createScheduleRequest struct {
	CronExpr string `json:"cron_expr"`
	Timezone string `json:"timezone"`
}

// updateScheduleRequest is the JSON body for PUT /api/v1/schedules/{scheduleID}.
type updateScheduleRequest struct {
	CronExpr string `json:"cron_expr"`
	Timezone string `json:"timezone"`
}

// cronParser is the shared cron expression parser.
var cronParser = cron.NewParser(
	cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// validateCronExpr returns an error if the expression is invalid.
func validateCronExpr(expr string) error {
	_, err := cronParser.Parse(expr)
	return err
}

// validateTimezone returns an error if the timezone string cannot be loaded.
func validateTimezone(tz string) error {
	if tz == "" {
		return nil
	}
	_, err := time.LoadLocation(tz)
	return err
}

// ListSchedules handles GET /api/v1/jobs/{jobID}/schedules.
func (h *ScheduleHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	if _, err := h.jobStore.GetJob(r.Context(), jobID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	schedules, err := h.store.ListSchedulesByJob(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if schedules == nil {
		schedules = []store.CronSchedule{}
	}
	writeJSON(w, http.StatusOK, schedules)
}

// CreateSchedule handles POST /api/v1/jobs/{jobID}/schedules.
func (h *ScheduleHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	var req createScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "cron_expr is required")
		return
	}

	if err := validateCronExpr(req.CronExpr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid cron_expr: "+err.Error())
		return
	}

	if err := validateTimezone(req.Timezone); err != nil {
		writeError(w, http.StatusBadRequest, "invalid timezone: "+err.Error())
		return
	}

	if _, err := h.jobStore.GetJob(r.Context(), jobID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	created, err := h.store.CreateSchedule(r.Context(), store.CreateScheduleParams{
		JobID:    jobID,
		CronExpr: req.CronExpr,
		Timezone: req.Timezone,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.scheduler.ReloadSchedule(r.Context(), created.ScheduleID); err != nil {
		slog.Warn("failed to reload schedule after create", "schedule_id", created.ScheduleID, "error", err)
	}

	writeJSON(w, http.StatusCreated, created)
}

// GetSchedule handles GET /api/v1/schedules/{scheduleID}.
func (h *ScheduleHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "scheduleID")

	sc, err := h.store.GetSchedule(r.Context(), scheduleID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

// UpdateSchedule handles PUT /api/v1/schedules/{scheduleID}.
func (h *ScheduleHandler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "scheduleID")

	if _, err := h.store.GetSchedule(r.Context(), scheduleID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req updateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "cron_expr is required")
		return
	}

	if err := validateCronExpr(req.CronExpr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid cron_expr: "+err.Error())
		return
	}

	if err := validateTimezone(req.Timezone); err != nil {
		writeError(w, http.StatusBadRequest, "invalid timezone: "+err.Error())
		return
	}

	updated, err := h.store.UpdateSchedule(r.Context(), store.UpdateScheduleParams{
		ScheduleID: scheduleID,
		CronExpr:   req.CronExpr,
		Timezone:   req.Timezone,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.scheduler.ReloadSchedule(r.Context(), scheduleID); err != nil {
		slog.Warn("failed to reload schedule after update", "schedule_id", scheduleID, "error", err)
	}

	writeJSON(w, http.StatusOK, updated)
}

// DeleteSchedule handles DELETE /api/v1/schedules/{scheduleID}.
func (h *ScheduleHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "scheduleID")

	if err := h.store.DeleteSchedule(r.Context(), scheduleID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.scheduler.RemoveSchedule(scheduleID)
	w.WriteHeader(http.StatusNoContent)
}

// PauseSchedule handles POST /api/v1/schedules/{scheduleID}/pause.
func (h *ScheduleHandler) PauseSchedule(w http.ResponseWriter, r *http.Request) {
	h.setEnabled(w, r, false)
}

// ResumeSchedule handles POST /api/v1/schedules/{scheduleID}/resume.
func (h *ScheduleHandler) ResumeSchedule(w http.ResponseWriter, r *http.Request) {
	h.setEnabled(w, r, true)
}

// setEnabled is a shared implementation for pause/resume.
func (h *ScheduleHandler) setEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	scheduleID := chi.URLParam(r, "scheduleID")

	if err := h.store.SetScheduleEnabled(r.Context(), scheduleID, enabled); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.scheduler.ReloadSchedule(r.Context(), scheduleID); err != nil {
		slog.Warn("failed to reload schedule after enable/disable", "schedule_id", scheduleID, "enabled", enabled, "error", err)
	}

	sc, err := h.store.GetSchedule(r.Context(), scheduleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, sc)
}
