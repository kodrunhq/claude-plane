// Package handler provides REST API handlers for the job system.
package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/claudeplane/claude-plane/internal/server/orchestrator"
	"github.com/claudeplane/claude-plane/internal/server/store"
)

// JobHandler handles REST endpoints for job and step CRUD.
type JobHandler struct {
	store store.JobStoreIface
}

// NewJobHandler creates a new JobHandler.
func NewJobHandler(s store.JobStoreIface) *JobHandler {
	return &JobHandler{store: s}
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

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
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

	job, err := h.store.CreateJob(r.Context(), req.Name, req.Description, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

// ListJobs handles GET /api/v1/jobs.
func (h *JobHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.store.ListJobs(r.Context())
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
	jobID := chi.URLParam(r, "jobID")
	detail, err := h.store.GetJob(r.Context(), jobID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
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
	jobID := chi.URLParam(r, "jobID")
	var req updateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	job, err := h.store.UpdateJob(r.Context(), jobID, req.Name, req.Description)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
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
	jobID := chi.URLParam(r, "jobID")
	err := h.store.DeleteJob(r.Context(), jobID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
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
	jobID := chi.URLParam(r, "jobID")
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

	step, err := h.store.CreateStep(r.Context(), jobID, req.Name, req.Prompt, req.MachineID, req.WorkingDir, req.Command, req.Args, req.TimeoutSeconds, req.SortOrder, req.OnFailure)
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
	stepID := chi.URLParam(r, "stepID")
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

	err := h.store.UpdateStep(r.Context(), stepID, req.Name, req.Prompt, req.MachineID, req.WorkingDir, req.Command, req.Args, req.TimeoutSeconds, req.SortOrder, req.OnFailure)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
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
	stepID := chi.URLParam(r, "stepID")
	err := h.store.DeleteStep(r.Context(), stepID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
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
	jobID := chi.URLParam(r, "jobID")
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

	// Add the dependency edge
	if err := h.store.AddDependency(r.Context(), stepID, req.DependsOn); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate DAG after adding
	steps, deps, err := h.store.GetStepsWithDeps(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := orchestrator.ValidateDAG(steps, deps); err != nil {
		// Cycle detected: roll back the edge
		_ = h.store.RemoveDependency(r.Context(), stepID, req.DependsOn)
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
	stepID := chi.URLParam(r, "stepID")
	depID := chi.URLParam(r, "depID")
	err := h.store.RemoveDependency(r.Context(), stepID, depID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "dependency not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
