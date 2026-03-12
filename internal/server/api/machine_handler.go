package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/claudeplane/claude-plane/internal/server/store"
	"github.com/go-chi/chi/v5"
)

// machineResponse is the JSON representation of a machine for API responses.
type machineResponse struct {
	MachineID   string     `json:"machine_id"`
	DisplayName string     `json:"display_name"`
	Status      string     `json:"status"`
	MaxSessions int32      `json:"max_sessions"`
	LastSeenAt  *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
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
		resp := machineResponse{
			MachineID:   m.MachineID,
			DisplayName: m.DisplayName,
			Status:      m.Status,
			MaxSessions: m.MaxSessions,
			LastSeenAt:  m.LastSeenAt,
			CreatedAt:   m.CreatedAt,
		}

		// Overlay live status from connection manager
		if agent := h.connMgr.GetAgent(m.MachineID); agent != nil {
			resp.Status = "connected"
			now := time.Now()
			resp.LastSeenAt = &now
		}

		result = append(result, resp)
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

	resp := machineResponse{
		MachineID:   machine.MachineID,
		DisplayName: machine.DisplayName,
		Status:      machine.Status,
		MaxSessions: machine.MaxSessions,
		LastSeenAt:  machine.LastSeenAt,
		CreatedAt:   machine.CreatedAt,
	}

	// Overlay live status from connection manager
	if agent := h.connMgr.GetAgent(machine.MachineID); agent != nil {
		resp.Status = "connected"
		now := time.Now()
		resp.LastSeenAt = &now
	}

	writeJSON(w, http.StatusOK, resp)
}
