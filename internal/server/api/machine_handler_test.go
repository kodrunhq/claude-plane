package api_test

import (
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
