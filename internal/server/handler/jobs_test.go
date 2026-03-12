package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/claudeplane/claude-plane/internal/server/handler"
	"github.com/claudeplane/claude-plane/internal/server/store"
)

// newTestStore creates an in-memory Store for testing.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newJobRouter creates a chi router with job routes for testing.
func newJobRouter(t *testing.T, s store.JobStoreIface) (*httptest.Server, store.JobStoreIface) {
	t.Helper()
	h := handler.NewJobHandler(s)
	r := chi.NewRouter()
	handler.RegisterJobRoutes(r, h)
	return httptest.NewServer(r), s
}

func TestJobHandler_CreateJob(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"name":        "My Job",
		"description": "A test job",
	})
	resp, err := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var result store.Job
	json.NewDecoder(resp.Body).Decode(&result)
	if result.JobID == "" {
		t.Error("expected job_id in response")
	}
	if result.Name != "My Job" {
		t.Errorf("expected name 'My Job', got %q", result.Name)
	}
}

func TestJobHandler_ListJobs(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	// Create two jobs
	for _, name := range []string{"Job A", "Job B"} {
		body, _ := json.Marshal(map[string]string{"name": name})
		resp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
		resp.Body.Close()
	}

	resp, err := http.Get(srv.URL + "/api/v1/jobs")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []store.Job
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(result))
	}
}

func TestJobHandler_GetJob(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	// Create a job
	body, _ := json.Marshal(map[string]string{"name": "Get Test"})
	createResp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	// Get the job
	resp, err := http.Get(srv.URL + "/api/v1/jobs/" + created.JobID)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var detail store.JobDetail
	json.NewDecoder(resp.Body).Decode(&detail)
	if detail.Job.Name != "Get Test" {
		t.Errorf("expected name 'Get Test', got %q", detail.Job.Name)
	}
}

func TestJobHandler_GetJob_NotFound(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/jobs/nonexistent-id")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestJobHandler_DeleteJob(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	// Create a job
	body, _ := json.Marshal(map[string]string{"name": "Delete Test"})
	createResp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	// Delete it
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/jobs/"+created.JobID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Verify it's gone
	getResp, _ := http.Get(srv.URL + "/api/v1/jobs/" + created.JobID)
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}

func TestJobHandler_AddStep(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	// Create a job
	body, _ := json.Marshal(map[string]string{"name": "Step Test"})
	createResp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	// Add a step
	stepBody, _ := json.Marshal(map[string]interface{}{
		"name":        "Step 1",
		"prompt":      "Do something",
		"machine_id":  "",
		"working_dir": "/tmp",
	})
	resp, err := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps", "application/json", bytes.NewReader(stepBody))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var step store.Step
	json.NewDecoder(resp.Body).Decode(&step)
	if step.StepID == "" {
		t.Error("expected step_id in response")
	}
	if step.Name != "Step 1" {
		t.Errorf("expected name 'Step 1', got %q", step.Name)
	}
}

func TestJobHandler_UpdateStep(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	// Create a job and step
	body, _ := json.Marshal(map[string]string{"name": "Update Step Test"})
	createResp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	stepBody, _ := json.Marshal(map[string]interface{}{"name": "Original", "prompt": "do thing"})
	stepResp, _ := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps", "application/json", bytes.NewReader(stepBody))
	var step store.Step
	json.NewDecoder(stepResp.Body).Decode(&step)
	stepResp.Body.Close()

	// Update the step
	updateBody, _ := json.Marshal(map[string]interface{}{"name": "Updated", "prompt": "do updated thing"})
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/jobs/"+created.JobID+"/steps/"+step.StepID, bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestJobHandler_DeleteStep(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	// Create a job and step
	body, _ := json.Marshal(map[string]string{"name": "Delete Step Test"})
	createResp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	stepBody, _ := json.Marshal(map[string]interface{}{"name": "ToDelete", "prompt": "do thing"})
	stepResp, _ := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps", "application/json", bytes.NewReader(stepBody))
	var step store.Step
	json.NewDecoder(stepResp.Body).Decode(&step)
	stepResp.Body.Close()

	// Delete the step
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/jobs/"+created.JobID+"/steps/"+step.StepID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestJobHandler_AddDependency(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	// Create a job with two steps
	body, _ := json.Marshal(map[string]string{"name": "Dep Test"})
	createResp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	step1Body, _ := json.Marshal(map[string]interface{}{"name": "Step 1", "prompt": "first"})
	step1Resp, _ := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps", "application/json", bytes.NewReader(step1Body))
	var step1 store.Step
	json.NewDecoder(step1Resp.Body).Decode(&step1)
	step1Resp.Body.Close()

	step2Body, _ := json.Marshal(map[string]interface{}{"name": "Step 2", "prompt": "second"})
	step2Resp, _ := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps", "application/json", bytes.NewReader(step2Body))
	var step2 store.Step
	json.NewDecoder(step2Resp.Body).Decode(&step2)
	step2Resp.Body.Close()

	// Add dependency: step2 depends on step1
	depBody, _ := json.Marshal(map[string]string{"depends_on": step1.StepID})
	resp, err := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps/"+step2.StepID+"/deps", "application/json", bytes.NewReader(depBody))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
}

func TestJobHandler_AddDependency_CycleRejected(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	// Create a job with two steps and a dependency
	body, _ := json.Marshal(map[string]string{"name": "Cycle Test"})
	createResp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	step1Body, _ := json.Marshal(map[string]interface{}{"name": "Step 1", "prompt": "first"})
	step1Resp, _ := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps", "application/json", bytes.NewReader(step1Body))
	var step1 store.Step
	json.NewDecoder(step1Resp.Body).Decode(&step1)
	step1Resp.Body.Close()

	step2Body, _ := json.Marshal(map[string]interface{}{"name": "Step 2", "prompt": "second"})
	step2Resp, _ := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps", "application/json", bytes.NewReader(step2Body))
	var step2 store.Step
	json.NewDecoder(step2Resp.Body).Decode(&step2)
	step2Resp.Body.Close()

	// Add step2 depends on step1
	depBody, _ := json.Marshal(map[string]string{"depends_on": step1.StepID})
	resp, _ := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps/"+step2.StepID+"/deps", "application/json", bytes.NewReader(depBody))
	resp.Body.Close()

	// Try to add step1 depends on step2 (cycle!)
	cycleBody, _ := json.Marshal(map[string]string{"depends_on": step2.StepID})
	cycleResp, err := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps/"+step1.StepID+"/deps", "application/json", bytes.NewReader(cycleBody))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer cycleResp.Body.Close()

	if cycleResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for cycle, got %d", cycleResp.StatusCode)
	}
}

func TestJobHandler_RemoveDependency(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	// Create job, steps, dependency
	body, _ := json.Marshal(map[string]string{"name": "Remove Dep Test"})
	createResp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	step1Body, _ := json.Marshal(map[string]interface{}{"name": "S1", "prompt": "first"})
	step1Resp, _ := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps", "application/json", bytes.NewReader(step1Body))
	var step1 store.Step
	json.NewDecoder(step1Resp.Body).Decode(&step1)
	step1Resp.Body.Close()

	step2Body, _ := json.Marshal(map[string]interface{}{"name": "S2", "prompt": "second"})
	step2Resp, _ := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps", "application/json", bytes.NewReader(step2Body))
	var step2 store.Step
	json.NewDecoder(step2Resp.Body).Decode(&step2)
	step2Resp.Body.Close()

	// Add dependency
	depBody, _ := json.Marshal(map[string]string{"depends_on": step1.StepID})
	depResp, _ := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps/"+step2.StepID+"/deps", "application/json", bytes.NewReader(depBody))
	depResp.Body.Close()

	// Remove dependency
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/jobs/"+created.JobID+"/steps/"+step2.StepID+"/deps/"+step1.StepID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}
