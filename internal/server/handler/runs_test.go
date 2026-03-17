package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/orchestrator"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// mockExecutor records step executions and completes them via channel.
type mockExecutor struct {
	mu       sync.Mutex
	executed []store.RunStep
	results  chan stepResult
}

type stepResult struct {
	stepID   string
	exitCode int
}

func newMockExecutor() *mockExecutor {
	return &mockExecutor{results: make(chan stepResult, 100)}
}

func (m *mockExecutor) ExecuteStep(ctx context.Context, rs store.RunStep, _ *orchestrator.ResolveContext, onComplete func(stepID string, exitCode int)) {
	m.mu.Lock()
	m.executed = append(m.executed, rs)
	m.mu.Unlock()

	go func() {
		select {
		case res := <-m.results:
			onComplete(res.stepID, res.exitCode)
		case <-ctx.Done():
		}
	}()
}

func (m *mockExecutor) completeStep(stepID string, exitCode int) {
	m.results <- stepResult{stepID: stepID, exitCode: exitCode}
}

// newRunRouter creates a chi router with both job and run routes for testing.
func newRunRouter(t *testing.T) (*httptest.Server, *store.Store, *orchestrator.Orchestrator, *mockExecutor) {
	t.Helper()
	s := newTestStore(t)
	seedUser(t, s, "admin-test", "admin")
	exec := newMockExecutor()
	orch := orchestrator.NewOrchestrator(context.Background(), s, exec)

	jh := handler.NewJobHandler(s, adminClaims())
	rh := handler.NewRunHandler(s, orch, adminClaims())

	r := chi.NewRouter()
	handler.RegisterJobRoutes(r, jh)
	handler.RegisterRunRoutes(r, rh)

	return httptest.NewServer(r), s, orch, exec
}

// createJobWithStep is a test helper that creates a job with one step via the API.
func createJobWithStep(t *testing.T, srvURL string) (jobID, stepID string) {
	t.Helper()
	// Create job
	body, _ := json.Marshal(map[string]string{"name": "Run Test Job"})
	resp, _ := http.Post(srvURL+"/api/v1/jobs", "application/json", bytes.NewReader(body))
	var job store.Job
	json.NewDecoder(resp.Body).Decode(&job)
	resp.Body.Close()

	// Add step
	stepBody, _ := json.Marshal(map[string]interface{}{"name": "Step 1", "prompt": "do work"})
	stepResp, _ := http.Post(srvURL+"/api/v1/jobs/"+job.JobID+"/steps", "application/json", bytes.NewReader(stepBody))
	var step store.Step
	json.NewDecoder(stepResp.Body).Decode(&step)
	stepResp.Body.Close()

	return job.JobID, step.StepID
}

func TestRunHandler_TriggerRun(t *testing.T) {
	srv, _, _, exec := newRunRouter(t)
	defer srv.Close()

	jobID, stepID := createJobWithStep(t, srv.URL)

	body, _ := json.Marshal(map[string]string{"trigger_type": "manual"})
	resp, err := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var run store.Run
	json.NewDecoder(resp.Body).Decode(&run)
	if run.RunID == "" {
		t.Error("expected run_id in response")
	}
	if run.JobID != jobID {
		t.Errorf("expected job_id %s, got %s", jobID, run.JobID)
	}

	// Complete the step to clean up the DAGRunner
	exec.completeStep(stepID, 0)
}

func TestRunHandler_ListRuns(t *testing.T) {
	srv, _, _, exec := newRunRouter(t)
	defer srv.Close()

	jobID, stepID := createJobWithStep(t, srv.URL)

	// Trigger a run
	body, _ := json.Marshal(map[string]string{"trigger_type": "manual"})
	triggerResp, _ := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body))
	triggerResp.Body.Close()
	exec.completeStep(stepID, 0)

	// List runs
	resp, err := http.Get(srv.URL + "/api/v1/runs?job_id=" + jobID)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var runs []store.RunWithJobName
	json.NewDecoder(resp.Body).Decode(&runs)
	if len(runs) != 1 {
		t.Errorf("expected 1 run, got %d", len(runs))
	}
}

func TestRunHandler_GetRun(t *testing.T) {
	srv, _, _, exec := newRunRouter(t)
	defer srv.Close()

	jobID, stepID := createJobWithStep(t, srv.URL)

	// Trigger a run
	body, _ := json.Marshal(map[string]string{"trigger_type": "manual"})
	triggerResp, _ := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body))
	var run store.Run
	json.NewDecoder(triggerResp.Body).Decode(&run)
	triggerResp.Body.Close()

	// Get run details
	resp, err := http.Get(srv.URL + "/api/v1/runs/" + run.RunID)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var detail store.RunDetail
	json.NewDecoder(resp.Body).Decode(&detail)
	if detail.Run.RunID != run.RunID {
		t.Errorf("expected run_id %s, got %s", run.RunID, detail.Run.RunID)
	}
	if len(detail.RunSteps) != 1 {
		t.Errorf("expected 1 run_step, got %d", len(detail.RunSteps))
	}

	exec.completeStep(stepID, 0)
}

func TestRunHandler_CancelRun(t *testing.T) {
	srv, _, _, _ := newRunRouter(t)
	defer srv.Close()

	jobID, _ := createJobWithStep(t, srv.URL)

	// Trigger a run (don't complete steps so it stays active)
	body, _ := json.Marshal(map[string]string{"trigger_type": "manual"})
	triggerResp, _ := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body))
	var run store.Run
	json.NewDecoder(triggerResp.Body).Decode(&run)
	triggerResp.Body.Close()

	// Cancel the run
	cancelResp, err := http.Post(srv.URL+"/api/v1/runs/"+run.RunID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer cancelResp.Body.Close()

	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", cancelResp.StatusCode)
	}

	// Verify run is cancelled
	getResp, _ := http.Get(srv.URL + "/api/v1/runs/" + run.RunID)
	var detail store.RunDetail
	json.NewDecoder(getResp.Body).Decode(&detail)
	getResp.Body.Close()

	if detail.Run.Status != "cancelled" {
		t.Errorf("expected status 'cancelled', got %q", detail.Run.Status)
	}
}

func TestRunHandler_RetryStep(t *testing.T) {
	srv, s, _, exec := newRunRouter(t)
	defer srv.Close()

	jobID, stepID := createJobWithStep(t, srv.URL)

	// Trigger a run
	body, _ := json.Marshal(map[string]string{"trigger_type": "manual"})
	triggerResp, _ := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body))
	var run store.Run
	json.NewDecoder(triggerResp.Body).Decode(&run)
	triggerResp.Body.Close()

	// Complete step with failure
	exec.completeStep(stepID, 1)

	// Wait a moment for the DAGRunner to process
	// Verify step is failed
	detail, _ := s.GetRunWithSteps(context.Background(), run.RunID)
	if detail == nil || len(detail.RunSteps) == 0 {
		t.Fatal("expected run steps")
	}

	// Retry the failed step
	retryResp, err := http.Post(srv.URL+"/api/v1/runs/"+run.RunID+"/steps/"+stepID+"/retry", "application/json", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer retryResp.Body.Close()

	if retryResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", retryResp.StatusCode)
	}

	// Complete the retried step
	exec.completeStep(stepID, 0)
}

func TestRunHandler_ListRuns_Global(t *testing.T) {
	srv, _, _, exec := newRunRouter(t)
	defer srv.Close()

	// Create two jobs each with one step.
	jobID1, stepID1 := createJobWithStep(t, srv.URL)
	jobID2, stepID2 := createJobWithStep(t, srv.URL)

	// Trigger a run on each job.
	triggerRun := func(jobID string) {
		body, _ := json.Marshal(map[string]string{"trigger_type": "manual"})
		resp, _ := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body))
		resp.Body.Close()
	}
	triggerRun(jobID1)
	exec.completeStep(stepID1, 0)
	triggerRun(jobID2)
	exec.completeStep(stepID2, 0)

	// List runs without job_id — should return runs from both jobs.
	resp, err := http.Get(srv.URL + "/api/v1/runs")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var runs []store.RunWithJobName
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("expected 2 runs across jobs, got %d", len(runs))
	}
	for _, r := range runs {
		if r.JobName == "" {
			t.Error("expected job_name to be populated in global listing")
		}
	}
}

func TestRunHandler_ListRuns_GlobalWithFilters(t *testing.T) {
	srv, _, _, exec := newRunRouter(t)
	defer srv.Close()

	jobID, stepID := createJobWithStep(t, srv.URL)

	// Trigger one manual run and complete it successfully.
	body, _ := json.Marshal(map[string]string{"trigger_type": "manual"})
	resp1, _ := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body))
	resp1.Body.Close()
	exec.completeStep(stepID, 0)

	// Trigger a second run but leave it running.
	body2, _ := json.Marshal(map[string]string{"trigger_type": "scheduled"})
	resp2, _ := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body2))
	resp2.Body.Close()
	// Don't complete — it stays in "running" state.

	// Filter by status=completed — should get 1 run.
	resp, err := http.Get(srv.URL + "/api/v1/runs?status=completed")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var completedRuns []store.RunWithJobName
	json.NewDecoder(resp.Body).Decode(&completedRuns)
	if len(completedRuns) != 1 {
		t.Errorf("expected 1 completed run, got %d", len(completedRuns))
	}

	// Filter by trigger_type=scheduled — should get 1 run.
	resp3, err := http.Get(srv.URL + "/api/v1/runs?trigger_type=scheduled")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}

	var scheduledRuns []store.RunWithJobName
	json.NewDecoder(resp3.Body).Decode(&scheduledRuns)
	if len(scheduledRuns) != 1 {
		t.Errorf("expected 1 scheduled run, got %d", len(scheduledRuns))
	}
	if scheduledRuns[0].TriggerType != "scheduled" {
		t.Errorf("expected trigger_type=scheduled, got %s", scheduledRuns[0].TriggerType)
	}
}

func TestRunHandler_ListRuns_WithJobID(t *testing.T) {
	srv, _, _, exec := newRunRouter(t)
	defer srv.Close()

	jobID1, stepID1 := createJobWithStep(t, srv.URL)
	jobID2, stepID2 := createJobWithStep(t, srv.URL)

	// Trigger one run per job.
	triggerAndComplete := func(jobID, stepID string) {
		body, _ := json.Marshal(map[string]string{"trigger_type": "manual"})
		resp, _ := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body))
		resp.Body.Close()
		exec.completeStep(stepID, 0)
	}
	triggerAndComplete(jobID1, stepID1)
	triggerAndComplete(jobID2, stepID2)

	// List runs for job1 only — should return []RunWithJobName, exactly 1.
	resp, err := http.Get(srv.URL + "/api/v1/runs?job_id=" + jobID1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var runs []store.RunWithJobName
	json.NewDecoder(resp.Body).Decode(&runs)
	if len(runs) != 1 {
		t.Errorf("expected 1 run for job1, got %d", len(runs))
	}
	if runs[0].JobID != jobID1 {
		t.Errorf("expected job_id=%s, got %s", jobID1, runs[0].JobID)
	}
	if runs[0].JobName == "" {
		t.Error("expected job_name to be populated when filtering by job_id")
	}
}

func TestRunHandler_ListRuns_ReturnEmptyArrayNotNull(t *testing.T) {
	srv, _, _, _ := newRunRouter(t)
	defer srv.Close()

	// Global list with no runs.
	resp, err := http.Get(srv.URL + "/api/v1/runs")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Ensure body is a JSON array, not null.
	var raw json.RawMessage
	json.NewDecoder(resp.Body).Decode(&raw)
	if string(raw) == "null" {
		t.Error("expected empty array [], got null")
	}
}

func TestRunHandler_ListRuns_LimitCapped(t *testing.T) {
	srv, _, _, _ := newRunRouter(t)
	defer srv.Close()

	// Requesting limit > 200 should not error — just return up to max.
	resp, err := http.Get(srv.URL + "/api/v1/runs?limit=9999")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for oversized limit, got %d", resp.StatusCode)
	}
}

func TestRunHandler_ListRuns_NonAdminScopedToOwnRuns(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "admin-test", "admin")
	seedUser(t, s, "user-A", "member")
	exec := newMockExecutor()
	orch := orchestrator.NewOrchestrator(context.Background(), s, exec)

	nonAdminClaims := func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "user-A", Role: "member"}
	}

	rh := handler.NewRunHandler(s, orch, nonAdminClaims)
	jh := handler.NewJobHandler(s, adminClaims())

	r := chi.NewRouter()
	handler.RegisterJobRoutes(r, jh)
	handler.RegisterRunRoutes(r, rh)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// Create a job (UserID will be "admin-test" from admin claims).
	jobID, stepID := createJobWithStep(t, srv.URL)

	// Trigger a run.
	body, _ := json.Marshal(map[string]string{"trigger_type": "manual"})
	triggerResp, _ := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body))
	triggerResp.Body.Close()
	exec.completeStep(stepID, 0)

	// List runs without job_id: non-admin should only see their own.
	resp, err := http.Get(srv.URL + "/api/v1/runs")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// The store was called with UserID="user-A", which scopes the results.
	// Since the job was created without a user_id, this should return empty.
	var runs []store.RunWithJobName
	json.NewDecoder(resp.Body).Decode(&runs)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs for user-A (scoped), got %d", len(runs))
	}
}

func TestRunHandler_ListRuns_AdminSeesAll(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "admin-1", "admin")
	exec := newMockExecutor()
	orch := orchestrator.NewOrchestrator(context.Background(), s, exec)

	adminClaimsLocal := func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "admin-1", Role: "admin"}
	}

	rh := handler.NewRunHandler(s, orch, adminClaimsLocal)
	jh := handler.NewJobHandler(s, adminClaimsLocal)

	r := chi.NewRouter()
	handler.RegisterJobRoutes(r, jh)
	handler.RegisterRunRoutes(r, rh)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// Create two jobs with runs.
	jobID1, stepID1 := createJobWithStep(t, srv.URL)
	jobID2, stepID2 := createJobWithStep(t, srv.URL)

	triggerAndComplete := func(jobID, stepID string) {
		body, _ := json.Marshal(map[string]string{"trigger_type": "manual"})
		resp, _ := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body))
		resp.Body.Close()
		exec.completeStep(stepID, 0)
	}
	triggerAndComplete(jobID1, stepID1)
	triggerAndComplete(jobID2, stepID2)

	// Admin lists all runs without job_id filter.
	resp, err := http.Get(srv.URL + "/api/v1/runs")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var runs []store.RunWithJobName
	json.NewDecoder(resp.Body).Decode(&runs)
	if len(runs) != 2 {
		t.Errorf("expected 2 runs for admin (sees all), got %d", len(runs))
	}
}

func TestRunHandler_RetryStep_NonFailed(t *testing.T) {
	srv, _, _, exec := newRunRouter(t)
	defer srv.Close()

	jobID, stepID := createJobWithStep(t, srv.URL)

	// Trigger a run
	body, _ := json.Marshal(map[string]string{"trigger_type": "manual"})
	triggerResp, _ := http.Post(srv.URL+"/api/v1/jobs/"+jobID+"/runs", "application/json", bytes.NewReader(body))
	var run store.Run
	json.NewDecoder(triggerResp.Body).Decode(&run)
	triggerResp.Body.Close()

	// Complete step successfully
	exec.completeStep(stepID, 0)

	// Try to retry a completed step - should return 400
	retryResp, err := http.Post(srv.URL+"/api/v1/runs/"+run.RunID+"/steps/"+stepID+"/retry", "application/json", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer retryResp.Body.Close()

	if retryResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for retry of non-failed step, got %d", retryResp.StatusCode)
	}
}
