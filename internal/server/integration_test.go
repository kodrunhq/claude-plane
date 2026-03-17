//go:build integration

package server_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kodrunhq/claude-plane/internal/server/testutil"
)

// TestAuthFlow verifies the full authentication lifecycle: register, login,
// JWT cookie is set, and the cookie grants access to a protected endpoint.
func TestAuthFlow(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Register
	resp := ts.AuthRequest(t, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email":        "flow@integration.test",
		"password":     "secure-password-123",
		"display_name": "Flow User",
	}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", resp.StatusCode)
	}

	// Login
	resp2 := ts.AuthRequest(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email":    "flow@integration.test",
		"password": "secure-password-123",
	}, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", resp2.StatusCode)
	}

	// Verify session cookie is set
	cookies := resp2.Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session_token" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session_token cookie after login")
	}
	if sessionCookie.HttpOnly != true {
		t.Error("expected session_token cookie to be HttpOnly")
	}

	// Access protected endpoint with cookie
	resp3 := ts.AuthRequest(t, http.MethodGet, "/api/v1/machines", nil, cookies)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("machines with cookie: expected 200, got %d", resp3.StatusCode)
	}

	// Access protected endpoint without cookie should fail
	resp4 := ts.AuthRequest(t, http.MethodGet, "/api/v1/machines", nil, nil)
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusUnauthorized {
		t.Fatalf("machines without cookie: expected 401, got %d", resp4.StatusCode)
	}
}

// TestTemplateCRUD verifies create, get, clone, and that the clone has a
// different ID from the original.
func TestTemplateCRUD(t *testing.T) {
	ts := testutil.NewTestServer(t)
	cookies := ts.LoginAsAdmin(t)

	// Create template
	createResp := ts.AuthRequest(t, http.MethodPost, "/api/v1/templates", map[string]string{
		"name":        "integ-template",
		"description": "integration test template",
	}, cookies)
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create template: expected 201, got %d", createResp.StatusCode)
	}

	var created map[string]interface{}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	templateID, ok := created["template_id"].(string)
	if !ok || templateID == "" {
		t.Fatal("expected non-empty template_id in create response")
	}

	// Get template
	getResp := ts.AuthRequest(t, http.MethodGet, "/api/v1/templates/"+templateID, nil, cookies)
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get template: expected 200, got %d", getResp.StatusCode)
	}

	var fetched map[string]interface{}
	if err := json.NewDecoder(getResp.Body).Decode(&fetched); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if fetched["name"] != "integ-template" {
		t.Errorf("get template name: expected integ-template, got %v", fetched["name"])
	}

	// Clone template
	cloneResp := ts.AuthRequest(t, http.MethodPost, "/api/v1/templates/"+templateID+"/clone", nil, cookies)
	defer cloneResp.Body.Close()
	if cloneResp.StatusCode != http.StatusCreated {
		t.Fatalf("clone template: expected 201, got %d", cloneResp.StatusCode)
	}

	var cloned map[string]interface{}
	if err := json.NewDecoder(cloneResp.Body).Decode(&cloned); err != nil {
		t.Fatalf("decode clone response: %v", err)
	}
	clonedID, ok := cloned["template_id"].(string)
	if !ok || clonedID == "" {
		t.Fatal("expected non-empty template_id in clone response")
	}
	if clonedID == templateID {
		t.Errorf("cloned template should have a different ID, got same: %s", clonedID)
	}
}

// TestJobLifecycle verifies creating a job, adding a step, listing jobs, and
// confirming the step count.
func TestJobLifecycle(t *testing.T) {
	ts := testutil.NewTestServer(t)
	cookies := ts.LoginAsAdmin(t)

	// Create job
	createResp := ts.AuthRequest(t, http.MethodPost, "/api/v1/jobs", map[string]string{
		"name":        "integ-job",
		"description": "integration test job",
	}, cookies)
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create job: expected 201, got %d", createResp.StatusCode)
	}

	var job map[string]interface{}
	if err := json.NewDecoder(createResp.Body).Decode(&job); err != nil {
		t.Fatalf("decode create job response: %v", err)
	}
	jobID, ok := job["job_id"].(string)
	if !ok || jobID == "" {
		t.Fatal("expected non-empty job_id")
	}

	// Add step
	stepResp := ts.AuthRequest(t, http.MethodPost, "/api/v1/jobs/"+jobID+"/steps", map[string]string{
		"name":   "step-1",
		"prompt": "do the thing",
	}, cookies)
	defer stepResp.Body.Close()
	if stepResp.StatusCode != http.StatusCreated {
		t.Fatalf("add step: expected 201, got %d", stepResp.StatusCode)
	}

	// List jobs and check step_count
	listResp := ts.AuthRequest(t, http.MethodGet, "/api/v1/jobs", nil, cookies)
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list jobs: expected 200, got %d", listResp.StatusCode)
	}

	var jobs []map[string]interface{}
	if err := json.NewDecoder(listResp.Body).Decode(&jobs); err != nil {
		t.Fatalf("decode list jobs response: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least 1 job in list")
	}

	found := false
	for _, j := range jobs {
		if j["job_id"] == jobID {
			found = true
			stepCount, _ := j["step_count"].(float64)
			if stepCount != 1 {
				t.Errorf("expected step_count 1, got %v", stepCount)
			}
			break
		}
	}
	if !found {
		t.Errorf("job %s not found in list response", jobID)
	}
}

// TestMachineList seeds a machine in the store and verifies it appears in
// GET /api/v1/machines.
func TestMachineList(t *testing.T) {
	ts := testutil.NewTestServer(t)
	cookies := ts.LoginAsAdmin(t)

	// Seed machine directly in the store
	machineID := testutil.MustCreateMachine(t, ts.Store, testutil.WithMachineID("integ-worker-1"))

	// GET /machines
	resp := ts.AuthRequest(t, http.MethodGet, "/api/v1/machines", nil, cookies)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list machines: expected 200, got %d", resp.StatusCode)
	}

	var machines []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&machines); err != nil {
		t.Fatalf("decode machines response: %v", err)
	}

	found := false
	for _, m := range machines {
		if m["machine_id"] == machineID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("machine %s not found in GET /machines response", machineID)
	}
}

// TestSessionMetadata creates a session with a model field via the store and
// verifies the metadata is returned from GET /api/v1/sessions/{id}.
func TestSessionMetadata(t *testing.T) {
	ts := testutil.NewTestServer(t)
	cookies := ts.LoginAsAdmin(t)

	// Seed machine and session in the store
	machineID := testutil.MustCreateMachine(t, ts.Store, testutil.WithMachineID("session-meta-worker"))
	sess := testutil.MustCreateSession(t, ts.Store, machineID,
		testutil.WithSessionModel("opus"),
		testutil.WithSessionCommand("claude"),
		testutil.WithSessionWorkingDir("/home/test"),
	)

	// GET /sessions/{id}
	resp := ts.AuthRequest(t, http.MethodGet, "/api/v1/sessions/"+sess.SessionID, nil, cookies)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get session: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode session response: %v", err)
	}

	if result["session_id"] != sess.SessionID {
		t.Errorf("expected session_id %s, got %v", sess.SessionID, result["session_id"])
	}
	if result["model"] != "opus" {
		t.Errorf("expected model opus, got %v", result["model"])
	}
	if result["machine_id"] != machineID {
		t.Errorf("expected machine_id %s, got %v", machineID, result["machine_id"])
	}
}
