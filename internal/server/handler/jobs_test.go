package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/store"
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

// adminClaims returns a ClaimsGetter that always returns admin claims.
func adminClaims() handler.ClaimsGetter {
	return func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "admin-test", Role: "admin"}
	}
}

// newJobRouter creates a chi router with job routes for testing.
// It seeds an admin user to satisfy foreign key constraints.
func newJobRouter(t *testing.T, s *store.Store) (*httptest.Server, *store.Store) {
	t.Helper()
	seedUser(t, s, "admin-test", "admin")
	h := handler.NewJobHandler(s, adminClaims())
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

	var result []store.JobWithStats
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

func TestJobHandler_UpdateJob(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	// Create a job
	body, _ := json.Marshal(map[string]string{"name": "Original", "description": "old desc"})
	createResp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	// Update the job
	updateBody, _ := json.Marshal(map[string]string{"name": "Updated", "description": "new desc"})
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/jobs/"+created.JobID, bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result store.Job
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %q", result.Name)
	}
	if result.Description != "new desc" {
		t.Errorf("expected description 'new desc', got %q", result.Description)
	}
}

func TestJobHandler_UpdateJob_NotFound(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	updateBody, _ := json.Marshal(map[string]string{"name": "Updated"})
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/jobs/nonexistent-id", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
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
	getResp, err := http.Get(srv.URL + "/api/v1/jobs/" + created.JobID)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}

func TestJobHandler_CloneJob(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	// Create a job with a step
	body, _ := json.Marshal(map[string]string{"name": "Clone Source", "description": "desc"})
	createResp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	stepBody, _ := json.Marshal(map[string]interface{}{"name": "S1", "prompt": "do thing"})
	stepResp, _ := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/steps", "application/json", bytes.NewReader(stepBody))
	stepResp.Body.Close()

	// Clone with default name
	resp, err := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/clone", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var cloned store.JobDetail
	json.NewDecoder(resp.Body).Decode(&cloned)
	if cloned.Job.JobID == "" {
		t.Error("expected job_id in response")
	}
	if cloned.Job.JobID == created.JobID {
		t.Error("cloned job should have different ID")
	}
	if cloned.Job.Name != "Clone Source (copy)" {
		t.Errorf("expected name 'Clone Source (copy)', got %q", cloned.Job.Name)
	}
	if len(cloned.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(cloned.Steps))
	}
}

func TestJobHandler_CloneJob_WithName(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"name": "Source"})
	createResp, _ := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	cloneBody, _ := json.Marshal(map[string]string{"name": "My Clone"})
	resp, err := http.Post(srv.URL+"/api/v1/jobs/"+created.JobID+"/clone", "application/json", bytes.NewReader(cloneBody))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var cloned store.JobDetail
	json.NewDecoder(resp.Body).Decode(&cloned)
	if cloned.Job.Name != "My Clone" {
		t.Errorf("expected name 'My Clone', got %q", cloned.Job.Name)
	}
}

func TestJobHandler_CloneJob_NotFound(t *testing.T) {
	s := newTestStore(t)
	srv, _ := newJobRouter(t, s)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/jobs/nonexistent-id/clone", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
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

func TestJobHandler_CreateJob_PublishesEvent(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "admin-test", "admin")
	pub := &mockPublisher{}

	h := handler.NewJobHandler(s, adminClaims())
	h.SetPublisher(pub)
	r := chi.NewRouter()
	handler.RegisterJobRoutes(r, h)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"name":        "Published Job",
		"description": "test event",
	})
	resp, err := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	events := pub.published()
	if len(events) < 1 {
		t.Fatalf("expected at least 1 published event, got %d", len(events))
	}
	evt := events[0]
	if evt.Type != "job.created" {
		t.Errorf("event type = %q, want %q", evt.Type, "job.created")
	}
	if evt.Payload["job_name"] != "Published Job" {
		t.Errorf("payload job_name = %v, want %q", evt.Payload["job_name"], "Published Job")
	}
}

func TestJobHandler_DeleteJob_PublishesEvent(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "admin-test", "admin")
	pub := &mockPublisher{}

	h := handler.NewJobHandler(s, adminClaims())
	h.SetPublisher(pub)
	r := chi.NewRouter()
	handler.RegisterJobRoutes(r, h)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// Create a job first.
	body, _ := json.Marshal(map[string]string{"name": "To Delete"})
	createResp, err := http.Post(srv.URL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	var created store.Job
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	// Reset captured events so we only see the delete event.
	pub.mu.Lock()
	pub.events = nil
	pub.mu.Unlock()

	// Delete the job.
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/jobs/"+created.JobID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	events := pub.published()
	if len(events) < 1 {
		t.Fatalf("expected at least 1 published event, got %d", len(events))
	}
	evt := events[0]
	if evt.Type != "job.deleted" {
		t.Errorf("event type = %q, want %q", evt.Type, "job.deleted")
	}
	if evt.Payload["job_name"] != "To Delete" {
		t.Errorf("payload job_name = %v, want %q", evt.Payload["job_name"], "To Delete")
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
