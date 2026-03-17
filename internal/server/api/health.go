package api

import (
	"encoding/json"
	"net/http"
)

// HealthzHandler returns an http.HandlerFunc that responds with 200 OK.
// This endpoint is unauthenticated for use by load balancers and orchestrators.
func HealthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
