package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/orchestrator"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

const (
	defaultListRunsLimit = 50
	maxListRunsLimit     = 200
)

// RunHandler handles REST endpoints for run management.
type RunHandler struct {
	store     store.JobStoreIface
	orch      *orchestrator.Orchestrator
	getClaims ClaimsGetter
}

// NewRunHandler creates a new RunHandler.
func NewRunHandler(s store.JobStoreIface, orch *orchestrator.Orchestrator, getClaims ClaimsGetter) *RunHandler {
	return &RunHandler{store: s, orch: orch, getClaims: getClaims}
}

// RegisterRunRoutes mounts all run-related routes on the given router.
func RegisterRunRoutes(r chi.Router, h *RunHandler) {
	r.Post("/api/v1/jobs/{jobID}/runs", h.TriggerRun)
	r.Get("/api/v1/runs", h.ListRuns)
	r.Get("/api/v1/runs/{runID}", h.GetRun)
	r.Post("/api/v1/runs/{runID}/cancel", h.CancelRun)
	r.Post("/api/v1/runs/{runID}/repair", h.RepairRun)
	r.Post("/api/v1/runs/{runID}/steps/{stepID}/retry", h.RetryStep)
}

// claims returns the current user's claims, or nil if no getter is configured.
func (h *RunHandler) claims(r *http.Request) *UserClaims {
	if h.getClaims == nil {
		return nil
	}
	return h.getClaims(r)
}

// authorizeJobAccess checks whether the current user can access the given job.
func (h *RunHandler) authorizeJobAccess(w http.ResponseWriter, r *http.Request, jobID string) bool {
	detail, err := h.store.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return false
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	c := h.claims(r)
	if c == nil || c.Role == "admin" || c.UserID == detail.Job.UserID {
		return true
	}
	writeError(w, http.StatusNotFound, "job not found")
	return false
}

// authorizeRunAccess checks whether the current user can access the given run
// (by checking ownership of its parent job).
func (h *RunHandler) authorizeRunAccess(w http.ResponseWriter, r *http.Request, runDetail *store.RunDetail) bool {
	c := h.claims(r)
	if c == nil || c.Role == "admin" {
		return true
	}
	jobDetail, err := h.store.GetJob(r.Context(), runDetail.Run.JobID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return false
	}
	if c.UserID == jobDetail.Job.UserID {
		return true
	}
	writeError(w, http.StatusNotFound, "run not found")
	return false
}

// triggerRunRequest is the JSON body for POST /api/v1/jobs/{jobID}/runs.
type triggerRunRequest struct {
	TriggerType   string            `json:"trigger_type"`
	TriggerDetail string            `json:"trigger_detail,omitempty"`
	Parameters    map[string]string `json:"parameters,omitempty"`
}

// TriggerRun handles POST /api/v1/jobs/{jobID}/runs.
// Creates a new run and starts DAG execution.
func (h *RunHandler) TriggerRun(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	if !h.authorizeJobAccess(w, r, jobID) {
		return
	}

	var req triggerRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TriggerType == "" {
		req.TriggerType = "manual"
	}

	run, err := h.orch.CreateRun(r.Context(), jobID, req.TriggerType, req.Parameters, req.TriggerDetail)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		if errors.Is(err, orchestrator.ErrMaxConcurrentRuns) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// ListRuns handles GET /api/v1/runs.
// All query params are optional: ?job_id, ?status, ?trigger_type, ?limit (default 50, max 200), ?offset (default 0).
// Always returns []RunWithJobName.
func (h *RunHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	jobID := q.Get("job_id")

	if jobID != "" {
		if !h.authorizeJobAccess(w, r, jobID) {
			return
		}
	}

	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = defaultListRunsLimit
	}
	if limit > maxListRunsLimit {
		limit = maxListRunsLimit
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}

	opts := store.ListRunsOptions{
		JobID:       jobID,
		Status:      q.Get("status"),
		TriggerType: q.Get("trigger_type"),
		Limit:       limit,
		Offset:      offset,
	}

	runs, err := h.store.ListAllRuns(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if runs == nil {
		runs = []store.RunWithJobName{}
	}
	writeJSON(w, http.StatusOK, runs)
}

// GetRun handles GET /api/v1/runs/{runID}.
// Returns run detail including run_steps.
func (h *RunHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	detail, err := h.store.GetRunWithSteps(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !h.authorizeRunAccess(w, r, detail) {
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// CancelRun handles POST /api/v1/runs/{runID}/cancel.
func (h *RunHandler) CancelRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	detail, err := h.store.GetRunWithSteps(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !h.authorizeRunAccess(w, r, detail) {
		return
	}

	if err := h.orch.CancelRun(r.Context(), runID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := h.store.GetRunWithSteps(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, updated.Run)
}

// RetryStep handles POST /api/v1/runs/{runID}/steps/{stepID}/retry.
func (h *RunHandler) RetryStep(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	stepID := chi.URLParam(r, "stepID")

	detail, err := h.store.GetRunWithSteps(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !h.authorizeRunAccess(w, r, detail) {
		return
	}

	var found bool
	for _, rs := range detail.RunSteps {
		if rs.StepID == stepID {
			found = true
			if rs.Status != store.StatusFailed && rs.Status != store.StatusSkipped && rs.Status != store.StatusCancelled {
				writeError(w, http.StatusBadRequest, "step is not in a retryable state (must be failed, skipped, or cancelled)")
				return
			}
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "step not found in run")
		return
	}

	if err := h.orch.RetryStep(r.Context(), runID, stepID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "retrying"})
}

// repairRunRequest is the optional JSON body for POST /api/v1/runs/{runID}/repair.
type repairRunRequest struct {
	Parameters map[string]string `json:"parameters,omitempty"`
}

// RepairRun handles POST /api/v1/runs/{runID}/repair.
// Resets all failed/skipped steps to pending and restarts the DAG.
func (h *RunHandler) RepairRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	detail, err := h.store.GetRunWithSteps(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !h.authorizeRunAccess(w, r, detail) {
		return
	}

	var req repairRunRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	if err := h.orch.RepairRun(r.Context(), runID, req.Parameters); err != nil {
		if errors.Is(err, orchestrator.ErrInvalidRunState) {
			writeError(w, http.StatusBadRequest, "can only repair failed or cancelled runs")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "repairing"})
}
