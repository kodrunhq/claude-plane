package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// --- mock store ---

type mockTriggerStore struct {
	triggers map[string]*store.JobTrigger
	jobNames map[string]string // jobID -> jobName
	err      error
}

func newMockTriggerStore() *mockTriggerStore {
	return &mockTriggerStore{
		triggers: make(map[string]*store.JobTrigger),
		jobNames: make(map[string]string),
	}
}

func (m *mockTriggerStore) CreateJobTrigger(_ context.Context, t store.JobTrigger) (*store.JobTrigger, error) {
	if m.err != nil {
		return nil, m.err
	}
	t.TriggerID = uuid.New().String()
	t.CreatedAt = time.Now().UTC()
	t.UpdatedAt = t.CreatedAt
	cp := t
	m.triggers[t.TriggerID] = &cp
	return &cp, nil
}

func (m *mockTriggerStore) ListJobTriggers(_ context.Context, jobID string) ([]store.JobTrigger, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []store.JobTrigger
	for _, t := range m.triggers {
		if t.JobID == jobID {
			result = append(result, *t)
		}
	}
	return result, nil
}

func (m *mockTriggerStore) UpdateJobTrigger(_ context.Context, triggerID, eventType, filter string) (*store.JobTrigger, error) {
	if m.err != nil {
		return nil, m.err
	}
	t, ok := m.triggers[triggerID]
	if !ok {
		return nil, store.ErrNotFound
	}
	updated := *t
	updated.EventType = eventType
	updated.Filter = filter
	updated.UpdatedAt = time.Now().UTC()
	m.triggers[triggerID] = &updated
	return &updated, nil
}

func (m *mockTriggerStore) ToggleJobTrigger(_ context.Context, triggerID string) (*store.JobTrigger, error) {
	if m.err != nil {
		return nil, m.err
	}
	t, ok := m.triggers[triggerID]
	if !ok {
		return nil, store.ErrNotFound
	}
	toggled := *t
	toggled.Enabled = !toggled.Enabled
	toggled.UpdatedAt = time.Now().UTC()
	m.triggers[triggerID] = &toggled
	return &toggled, nil
}

func (m *mockTriggerStore) GetJobTrigger(_ context.Context, triggerID string) (*store.JobTrigger, error) {
	if m.err != nil {
		return nil, m.err
	}
	t, ok := m.triggers[triggerID]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (m *mockTriggerStore) ListAllTriggers(_ context.Context, _ string) ([]store.JobTriggerWithJob, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []store.JobTriggerWithJob
	for _, t := range m.triggers {
		jobName := m.jobNames[t.JobID]
		if jobName == "" {
			jobName = "job-" + t.JobID
		}
		result = append(result, store.JobTriggerWithJob{
			JobTrigger: *t,
			JobName:    jobName,
		})
	}
	return result, nil
}

func (m *mockTriggerStore) DeleteJobTrigger(_ context.Context, triggerID string) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.triggers[triggerID]; !ok {
		return store.ErrNotFound
	}
	delete(m.triggers, triggerID)
	return nil
}

// --- router helper ---

func newTriggerRouter(h *handler.TriggerHandler) *httptest.Server {
	r := chi.NewRouter()
	handler.RegisterTriggerRoutes(r, h)
	return httptest.NewServer(r)
}

// --- tests ---

func TestTriggerHandler_ListTriggers_Empty(t *testing.T) {
	mock := newMockTriggerStore()
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/jobs/job-1/triggers")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []store.JobTrigger
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 triggers, got %d", len(result))
	}
}

func TestTriggerHandler_CreateTrigger(t *testing.T) {
	mock := newMockTriggerStore()
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	body := map[string]interface{}{
		"event_type": "run.completed",
	}

	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/v1/jobs/job-1/triggers", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created store.JobTrigger
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.TriggerID == "" {
		t.Error("expected non-empty trigger_id")
	}
	if created.JobID != "job-1" {
		t.Errorf("JobID = %q, want %q", created.JobID, "job-1")
	}
	if created.EventType != "run.completed" {
		t.Errorf("EventType = %q, want %q", created.EventType, "run.completed")
	}
	if !created.Enabled {
		t.Error("expected Enabled = true")
	}
}

func TestTriggerHandler_CreateTrigger_WithFilter(t *testing.T) {
	mock := newMockTriggerStore()
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	body := map[string]interface{}{
		"event_type": "run.*",
		"filter":     `{"status":"completed"}`,
	}

	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/v1/jobs/job-2/triggers", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created store.JobTrigger
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Filter != `{"status":"completed"}` {
		t.Errorf("Filter = %q, want %q", created.Filter, `{"status":"completed"}`)
	}
}

func TestTriggerHandler_CreateTrigger_MissingEventType(t *testing.T) {
	mock := newMockTriggerStore()
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	body := map[string]interface{}{}
	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/v1/jobs/job-1/triggers", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTriggerHandler_CreateTrigger_InvalidBody(t *testing.T) {
	mock := newMockTriggerStore()
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/jobs/job-1/triggers", "application/json", bytes.NewBufferString("not-json"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTriggerHandler_CreateTrigger_StoreError(t *testing.T) {
	mock := newMockTriggerStore()
	mock.err = context.DeadlineExceeded
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	body := map[string]interface{}{"event_type": "run.completed"}
	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/v1/jobs/job-1/triggers", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestTriggerHandler_ListTriggers_ReturnsJobTriggers(t *testing.T) {
	mock := newMockTriggerStore()
	// Pre-insert two triggers for job-1 and one for job-2.
	mock.triggers["t1"] = &store.JobTrigger{
		TriggerID: "t1", JobID: "job-1", EventType: "run.completed",
		Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	mock.triggers["t2"] = &store.JobTrigger{
		TriggerID: "t2", JobID: "job-1", EventType: "run.failed",
		Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	mock.triggers["t3"] = &store.JobTrigger{
		TriggerID: "t3", JobID: "job-2", EventType: "run.completed",
		Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}

	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/jobs/job-1/triggers")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []store.JobTrigger
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 triggers for job-1, got %d", len(result))
	}
}

func TestTriggerHandler_ListTriggers_StoreError(t *testing.T) {
	mock := newMockTriggerStore()
	mock.err = context.DeadlineExceeded
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/jobs/job-1/triggers")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestTriggerHandler_DeleteTrigger(t *testing.T) {
	mock := newMockTriggerStore()
	triggerID := uuid.New().String()
	mock.triggers[triggerID] = &store.JobTrigger{
		TriggerID: triggerID, JobID: "job-1", EventType: "run.completed",
		Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}

	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/triggers/"+triggerID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
	if _, ok := mock.triggers[triggerID]; ok {
		t.Error("trigger still exists after delete")
	}
}

func TestTriggerHandler_DeleteTrigger_NotFound(t *testing.T) {
	mock := newMockTriggerStore()
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/triggers/ghost", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestTriggerHandler_DeleteTrigger_StoreError(t *testing.T) {
	mock := newMockTriggerStore()
	mock.err = context.DeadlineExceeded
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/triggers/any-id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

// --- UpdateTrigger tests ---

func TestTriggerHandler_UpdateTrigger(t *testing.T) {
	mock := newMockTriggerStore()
	triggerID := uuid.New().String()
	mock.triggers[triggerID] = &store.JobTrigger{
		TriggerID: triggerID, JobID: "job-1", EventType: "run.completed",
		Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}

	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	body := map[string]interface{}{
		"event_type": "run.failed",
		"filter":     `{"job_id":"abc"}`,
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/triggers/"+triggerID, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var updated store.JobTrigger
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.EventType != "run.failed" {
		t.Errorf("EventType = %q, want %q", updated.EventType, "run.failed")
	}
	if updated.Filter != `{"job_id":"abc"}` {
		t.Errorf("Filter = %q, want %q", updated.Filter, `{"job_id":"abc"}`)
	}
}

func TestTriggerHandler_UpdateTrigger_NotFound(t *testing.T) {
	mock := newMockTriggerStore()
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	body := map[string]interface{}{"event_type": "run.completed"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/triggers/ghost", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestTriggerHandler_UpdateTrigger_MissingEventType(t *testing.T) {
	mock := newMockTriggerStore()
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	body := map[string]interface{}{"filter": "{}"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/triggers/any-id", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTriggerHandler_UpdateTrigger_InvalidBody(t *testing.T) {
	mock := newMockTriggerStore()
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/triggers/any-id", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTriggerHandler_UpdateTrigger_StoreError(t *testing.T) {
	mock := newMockTriggerStore()
	mock.err = context.DeadlineExceeded
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	body := map[string]interface{}{"event_type": "run.completed"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/triggers/any-id", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

// --- ToggleTrigger tests ---

func TestTriggerHandler_ToggleTrigger(t *testing.T) {
	mock := newMockTriggerStore()
	triggerID := uuid.New().String()
	mock.triggers[triggerID] = &store.JobTrigger{
		TriggerID: triggerID, JobID: "job-1", EventType: "run.completed",
		Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}

	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/triggers/"+triggerID+"/toggle", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST toggle: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var toggled store.JobTrigger
	if err := json.NewDecoder(resp.Body).Decode(&toggled); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if toggled.Enabled {
		t.Error("expected Enabled = false after toggle")
	}

	// Toggle again — should be true
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/triggers/"+triggerID+"/toggle", nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("POST toggle 2: %v", err)
	}
	defer resp2.Body.Close()

	var toggled2 store.JobTrigger
	if err := json.NewDecoder(resp2.Body).Decode(&toggled2); err != nil {
		t.Fatalf("decode 2: %v", err)
	}
	if !toggled2.Enabled {
		t.Error("expected Enabled = true after second toggle")
	}
}

func TestTriggerHandler_ToggleTrigger_NotFound(t *testing.T) {
	mock := newMockTriggerStore()
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/triggers/ghost/toggle", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST toggle: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestTriggerHandler_ToggleTrigger_StoreError(t *testing.T) {
	mock := newMockTriggerStore()
	mock.err = context.DeadlineExceeded
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/triggers/any-id/toggle", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST toggle: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

// --- ListAllTriggers tests ---

func TestTriggerHandler_ListAllTriggers(t *testing.T) {
	mock := newMockTriggerStore()
	mock.triggers["t1"] = &store.JobTrigger{
		TriggerID: "t1", JobID: "job-1", EventType: "run.completed",
		Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	mock.triggers["t2"] = &store.JobTrigger{
		TriggerID: "t2", JobID: "job-2", EventType: "run.failed",
		Enabled: false, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	mock.jobNames["job-1"] = "deploy-prod"
	mock.jobNames["job-2"] = "run-tests"

	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/triggers")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var triggers []struct {
		TriggerID string `json:"trigger_id"`
		JobName   string `json:"job_name"`
		EventType string `json:"event_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&triggers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(triggers) != 2 {
		t.Fatalf("expected 2 triggers, got %d", len(triggers))
	}

	// Verify job_name is populated for each trigger.
	namesByID := make(map[string]string)
	for _, tr := range triggers {
		namesByID[tr.TriggerID] = tr.JobName
	}
	if namesByID["t1"] != "deploy-prod" {
		t.Errorf("trigger t1 job_name = %q, want %q", namesByID["t1"], "deploy-prod")
	}
	if namesByID["t2"] != "run-tests" {
		t.Errorf("trigger t2 job_name = %q, want %q", namesByID["t2"], "run-tests")
	}
}

func TestTriggerHandler_ListAllTriggers_Empty(t *testing.T) {
	mock := newMockTriggerStore()
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/triggers")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var triggers []store.JobTriggerWithJob
	if err := json.NewDecoder(resp.Body).Decode(&triggers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(triggers) != 0 {
		t.Errorf("expected 0 triggers, got %d", len(triggers))
	}
}

func TestTriggerHandler_ListAllTriggers_StoreError(t *testing.T) {
	mock := newMockTriggerStore()
	mock.err = context.DeadlineExceeded
	h := handler.NewTriggerHandler(mock)
	srv := newTriggerRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/triggers")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}
