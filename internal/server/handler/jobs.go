// Package handler provides REST API handlers for the job system.
package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/httputil"
	"github.com/kodrunhq/claude-plane/internal/server/orchestrator"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// JobHandler handles REST endpoints for job and step CRUD.
type JobHandler struct {
	store     store.JobStoreIface
	getClaims ClaimsGetter
}

// NewJobHandler creates a new JobHandler.
func NewJobHandler(s store.JobStoreIface, getClaims ClaimsGetter) *JobHandler {
	return &JobHandler{store: s, getClaims: getClaims}
}

// claims returns the current user's claims, or nil if no getter is configured.
func (h *JobHandler) claims(r *http.Request) *UserClaims {
	if h.getClaims == nil {
		return nil
	}
	return h.getClaims(r)
}

// authorizeJob checks whether the current user can access the given job.
func (h *JobHandler) authorizeJob(w http.ResponseWriter, r *http.Request, job *store.Job) bool {
	c := h.claims(r)
	if c == nil || c.Role == "admin" || c.UserID == job.UserID {
		return true
	}
	writeError(w, http.StatusNotFound, "job not found")
	return false
}

// authorizeJobByID fetches the job and checks authorization in one step.
func (h *JobHandler) authorizeJobByID(w http.ResponseWriter, r *http.Request) *store.JobDetail {
	jobID := chi.URLParam(r, "jobID")
	detail, err := h.store.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return nil
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil
	}
	if !h.authorizeJob(w, r, &detail.Job) {
		return nil
	}
	return detail
}

// jobOwnsStep returns true if the given step ID belongs to the job in detail.
func jobOwnsStep(detail *store.JobDetail, stepID string) bool {
	for _, s := range detail.Steps {
		if s.StepID == stepID {
			return true
		}
	}
	return false
}

// RegisterJobRoutes mounts all job-related routes on the given router.
func RegisterJobRoutes(r chi.Router, h *JobHandler) {
	r.Post("/api/v1/jobs", h.CreateJob)
	r.Get("/api/v1/jobs", h.ListJobs)
	r.Get("/api/v1/jobs/{jobID}", h.GetJob)
	r.Put("/api/v1/jobs/{jobID}", h.UpdateJob)
	r.Delete("/api/v1/jobs/{jobID}", h.DeleteJob)

	r.Post("/api/v1/jobs/{jobID}/steps", h.AddStep)
	r.Put("/api/v1/jobs/{jobID}/steps/{stepID}", h.UpdateStep)
	r.Delete("/api/v1/jobs/{jobID}/steps/{stepID}", h.DeleteStep)

	r.Post("/api/v1/jobs/{jobID}/steps/{stepID}/deps", h.AddDependency)
	r.Delete("/api/v1/jobs/{jobID}/steps/{stepID}/deps/{depID}", h.RemoveDependency)
}

// writeJSON delegates to the shared httputil.WriteJSON helper.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	httputil.WriteJSON(w, status, data)
}

// writeError delegates to the shared httputil.WriteError helper.
func writeError(w http.ResponseWriter, status int, message string) {
	httputil.WriteError(w, status, message)
}

// createJobRequest is the JSON body for POST /api/v1/jobs.
type createJobRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateJob handles POST /api/v1/jobs.
func (h *JobHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	userID := ""
	if c := h.claims(r); c != nil {
		userID = c.UserID
	}

	job, err := h.store.CreateJob(r.Context(), req.Name, req.Description, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

// ListJobs handles GET /api/v1/jobs.
func (h *JobHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)

	var jobs []store.Job
	var err error
	if c != nil && c.Role != "admin" {
		jobs, err = h.store.ListJobsByUser(r.Context(), c.UserID)
	} else {
		jobs, err = h.store.ListJobs(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if jobs == nil {
		jobs = []store.Job{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

// GetJob handles GET /api/v1/jobs/{jobID}.
func (h *JobHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	detail := h.authorizeJobByID(w, r)
	if detail == nil {
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// updateJobRequest is the JSON body for PUT /api/v1/jobs/{jobID}.
type updateJobRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateJob handles PUT /api/v1/jobs/{jobID}.
func (h *JobHandler) UpdateJob(w http.ResponseWriter, r *http.Request) {
	detail := h.authorizeJobByID(w, r)
	if detail == nil {
		return
	}
	var req updateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	job, err := h.store.UpdateJob(r.Context(), detail.Job.JobID, req.Name, req.Description)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// DeleteJob handles DELETE /api/v1/jobs/{jobID}.
func (h *JobHandler) DeleteJob(w http.ResponseWriter, r *http.Request) {
	detail := h.authorizeJobByID(w, r)
	if detail == nil {
		return
	}
	err := h.store.DeleteJob(r.Context(), detail.Job.JobID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// addStepRequest is the JSON body for POST /api/v1/jobs/{jobID}/steps.
type addStepRequest struct {
	Name           string `json:"name"`
	Prompt         string `json:"prompt"`
	MachineID      string `json:"machine_id"`
	WorkingDir     string `json:"working_dir"`
	Command        string `json:"command"`
	Args           string `json:"args"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	SortOrder      int    `json:"sort_order"`
	OnFailure      string `json:"on_failure"`
}

// AddStep handles POST /api/v1/jobs/{jobID}/steps.
func (h *JobHandler) AddStep(w http.ResponseWriter, r *http.Request) {
	detail := h.authorizeJobByID(w, r)
	if detail == nil {
		return
	}
	var req addStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.OnFailure == "" {
		req.OnFailure = "fail_run"
	}
	if req.Command == "" {
		req.Command = "claude"
	}

	step, err := h.store.CreateStep(r.Context(), store.CreateStepParams{
		JobID: detail.Job.JobID, Name: req.Name, Prompt: req.Prompt,
		MachineID: req.MachineID, WorkingDir: req.WorkingDir,
		Command: req.Command, Args: req.Args,
		TimeoutSeconds: req.TimeoutSeconds, SortOrder: req.SortOrder,
		OnFailure: req.OnFailure,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, step)
}

// updateStepRequest is the JSON body for PUT /api/v1/jobs/{jobID}/steps/{stepID}.
type updateStepRequest struct {
	Name           string `json:"name"`
	Prompt         string `json:"prompt"`
	MachineID      string `json:"machine_id"`
	WorkingDir     string `json:"working_dir"`
	Command        string `json:"command"`
	Args           string `json:"args"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	SortOrder      int    `json:"sort_order"`
	OnFailure      string `json:"on_failure"`
}

// UpdateStep handles PUT /api/v1/jobs/{jobID}/steps/{stepID}.
func (h *JobHandler) UpdateStep(w http.ResponseWriter, r *http.Request) {
	detail := h.authorizeJobByID(w, r)
	if detail == nil {
		return
	}
	stepID := chi.URLParam(r, "stepID")
	if !jobOwnsStep(detail, stepID) {
		writeError(w, http.StatusNotFound, "step not found")
		return
	}
	var req updateStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.OnFailure == "" {
		req.OnFailure = "fail_run"
	}
	if req.Command == "" {
		req.Command = "claude"
	}

	err := h.store.UpdateStep(r.Context(), store.UpdateStepParams{
		StepID: stepID, Name: req.Name, Prompt: req.Prompt,
		MachineID: req.MachineID, WorkingDir: req.WorkingDir,
		Command: req.Command, Args: req.Args,
		TimeoutSeconds: req.TimeoutSeconds, SortOrder: req.SortOrder,
		OnFailure: req.OnFailure,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "step not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteStep handles DELETE /api/v1/jobs/{jobID}/steps/{stepID}.
func (h *JobHandler) DeleteStep(w http.ResponseWriter, r *http.Request) {
	detail := h.authorizeJobByID(w, r)
	if detail == nil {
		return
	}
	stepID := chi.URLParam(r, "stepID")
	if !jobOwnsStep(detail, stepID) {
		writeError(w, http.StatusNotFound, "step not found")
		return
	}
	err := h.store.DeleteStep(r.Context(), stepID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "step not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// addDependencyRequest is the JSON body for POST /api/v1/jobs/{jobID}/steps/{stepID}/deps.
type addDependencyRequest struct {
	DependsOn string `json:"depends_on"`
}

// AddDependency handles POST /api/v1/jobs/{jobID}/steps/{stepID}/deps.
// After adding the edge, validates the DAG. If a cycle is detected, removes
// the edge and returns 400.
func (h *JobHandler) AddDependency(w http.ResponseWriter, r *http.Request) {
	detail := h.authorizeJobByID(w, r)
	if detail == nil {
		return
	}
	stepID := chi.URLParam(r, "stepID")
	var req addDependencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DependsOn == "" {
		writeError(w, http.StatusBadRequest, "depends_on is required")
		return
	}

	if !jobOwnsStep(detail, stepID) || !jobOwnsStep(detail, req.DependsOn) {
		writeError(w, http.StatusNotFound, "step not found in this job")
		return
	}

	// Add the dependency edge
	if err := h.store.AddDependency(r.Context(), stepID, req.DependsOn); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate DAG after adding
	steps, deps, err := h.store.GetStepsWithDeps(r.Context(), detail.Job.JobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := orchestrator.ValidateDAG(steps, deps); err != nil {
		// Cycle detected: roll back the edge
		if err := h.store.RemoveDependency(r.Context(), stepID, req.DependsOn); err != nil {
			slog.Warn("failed to roll back dependency after cycle detection", "error", err, "step_id", stepID, "depends_on", req.DependsOn)
		}
		writeError(w, http.StatusBadRequest, "cycle detected in job DAG")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"step_id":    stepID,
		"depends_on": req.DependsOn,
	})
}

// RemoveDependency handles DELETE /api/v1/jobs/{jobID}/steps/{stepID}/deps/{depID}.
func (h *JobHandler) RemoveDependency(w http.ResponseWriter, r *http.Request) {
	detail := h.authorizeJobByID(w, r)
	if detail == nil {
		return
	}
	stepID := chi.URLParam(r, "stepID")
	depID := chi.URLParam(r, "depID")
	if !jobOwnsStep(detail, stepID) || !jobOwnsStep(detail, depID) {
		writeError(w, http.StatusNotFound, "step not found in this job")
		return
	}
	err := h.store.RemoveDependency(r.Context(), stepID, depID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "dependency not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
