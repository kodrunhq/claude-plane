package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// machineResponse is the JSON representation of a machine for API responses.
type machineResponse struct {
	MachineID      string              `json:"machine_id"`
	DisplayName    string              `json:"display_name"`
	Status         string              `json:"status"`
	MaxSessions    int32               `json:"max_sessions"`
	LastSeenAt     *time.Time          `json:"last_seen_at,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	Health         *machineHealthResponse `json:"health,omitempty"`
}

type machineHealthResponse struct {
	CPUCores       int32 `json:"cpu_cores"`
	MemoryTotalMB  int64 `json:"memory_total_mb"`
	MemoryUsedMB   int64 `json:"memory_used_mb"`
	ActiveSessions int32 `json:"active_sessions"`
	MaxSessions    int32 `json:"max_sessions"`
}

// buildMachineResponse constructs a machineResponse from a store.Machine,
// overlaying live connection status and health data from the connection manager.
func (h *Handlers) buildMachineResponse(m store.Machine) machineResponse {
	resp := machineResponse{
		MachineID:   m.MachineID,
		DisplayName: m.DisplayName,
		Status:      m.Status,
		MaxSessions: m.MaxSessions,
		LastSeenAt:  m.LastSeenAt,
		CreatedAt:   m.CreatedAt,
	}

	if agent := h.connMgr.GetAgent(m.MachineID); agent != nil {
		resp.Status = "connected"
		now := time.Now()
		resp.LastSeenAt = &now
		if hi := agent.GetHealth(); hi != nil {
			resp.Health = &machineHealthResponse{
				CPUCores:       hi.CPUCores,
				MemoryTotalMB:  hi.MemoryTotalMB,
				MemoryUsedMB:   hi.MemoryUsedMB,
				ActiveSessions: hi.ActiveSessions,
				MaxSessions:    hi.MaxSessions,
			}
		}
	}

	return resp
}

// ListMachines handles GET /api/v1/machines.
// Returns all machines with live status overlay from connection manager.
func (h *Handlers) ListMachines(w http.ResponseWriter, r *http.Request) {
	machines, err := h.store.ListMachines()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	result := make([]machineResponse, 0, len(machines))
	for _, m := range machines {
		result = append(result, h.buildMachineResponse(m))
	}

	writeJSON(w, http.StatusOK, result)
}

// GetMachine handles GET /api/v1/machines/{machineID}.
// Returns a single machine with live status overlay.
func (h *Handlers) GetMachine(w http.ResponseWriter, r *http.Request) {
	machineID := chi.URLParam(r, "machineID")

	machine, err := h.store.GetMachine(machineID)
	if err != nil {
		if errors.Is(err, store.ErrMachineNotFound) {
			writeError(w, http.StatusNotFound, "machine not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, h.buildMachineResponse(*machine))
}

// updateMachineRequest is the JSON body for PUT /api/v1/machines/{machineID}.
type updateMachineRequest struct {
	DisplayName *string `json:"display_name"`
}

// UpdateMachine handles PUT /api/v1/machines/{machineID}.
// Updates mutable machine fields (currently display_name). Admin-only.
func (h *Handlers) UpdateMachine(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r)
	if claims == nil || claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	machineID := chi.URLParam(r, "machineID")

	var req updateMachineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DisplayName == nil {
		writeError(w, http.StatusBadRequest, "display_name is required")
		return
	}

	trimmed := strings.TrimSpace(*req.DisplayName)
	if trimmed == "" {
		writeError(w, http.StatusBadRequest, "display_name must not be empty")
		return
	}

	if len(trimmed) > 255 {
		writeError(w, http.StatusBadRequest, "display_name must be 255 characters or fewer")
		return
	}

	if err := h.store.UpdateMachineDisplayName(machineID, trimmed); err != nil {
		if errors.Is(err, store.ErrMachineNotFound) {
			writeError(w, http.StatusNotFound, "machine not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	// Return the updated machine.
	machine, err := h.store.GetMachine(machineID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, h.buildMachineResponse(*machine))
}

// DeleteMachine handles DELETE /api/v1/machines/{machineID}.
// Soft-deletes the machine. Admin-only. Rejects if machine is currently connected.
func (h *Handlers) DeleteMachine(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r)
	if claims == nil || claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	machineID := chi.URLParam(r, "machineID")

	if agent := h.connMgr.GetAgent(machineID); agent != nil {
		writeError(w, http.StatusConflict, "cannot delete connected machine")
		return
	}

	if err := h.store.SoftDeleteMachine(machineID); err != nil {
		if errors.Is(err, store.ErrMachineNotFound) {
			writeError(w, http.StatusNotFound, "machine not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
