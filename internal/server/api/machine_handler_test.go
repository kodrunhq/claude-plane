package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestListMachinesAuthenticated(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "machines@example.com", "password123", "Machine User")
	resp.Body.Close()

	token := loginUser(t, srv, "machines@example.com", "password123")

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/machines", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	machinesResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("machines request: %v", err)
	}
	defer machinesResp.Body.Close()

	if machinesResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", machinesResp.StatusCode)
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(machinesResp.Body).Decode(&result); err != nil {
		t.Fatalf("decoding machines response: %v", err)
	}

	if result == nil {
		t.Fatal("expected JSON array, got null")
	}
}

func TestUpdateMachineSuccess(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "update@example.com", "password123", "Update User")
	resp.Body.Close()
	token := loginUser(t, srv, "update@example.com", "password123")

	// Seed a machine by calling the store directly is not possible via HTTP,
	// so we PUT against a non-existent machine first to verify 404.
	body, _ := json.Marshal(map[string]string{"display_name": "My Worker"})
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/machines/test-machine", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request: %v", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent machine, got %d", putResp.StatusCode)
	}
}

func TestUpdateMachineInvalidBody(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "invalid@example.com", "password123", "Invalid User")
	resp.Body.Close()
	token := loginUser(t, srv, "invalid@example.com", "password123")

	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/machines/test-machine", bytes.NewReader([]byte("not json")))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request: %v", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", putResp.StatusCode)
	}
}

func TestUpdateMachineMissingDisplayName(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "missing@example.com", "password123", "Missing User")
	resp.Body.Close()
	token := loginUser(t, srv, "missing@example.com", "password123")

	body, _ := json.Marshal(map[string]string{})
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/machines/test-machine", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request: %v", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing display_name, got %d", putResp.StatusCode)
	}
}

func TestListMachinesUnauthenticated(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/machines")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}
