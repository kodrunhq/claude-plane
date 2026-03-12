package api

import "net/http"

// ListMachines handles GET /api/v1/machines.
// Returns all machines with live status overlay from connection manager.
func (h *Handlers) ListMachines(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented")
}

// GetMachine handles GET /api/v1/machines/{machineID}.
// Returns a single machine with live status overlay.
func (h *Handlers) GetMachine(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented")
}
