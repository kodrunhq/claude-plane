package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// --- mock stores ---

type mockScheduleCRUDStore struct {
	schedules map[string]*store.CronSchedule
	err       error
}

func newMockScheduleCRUDStore() *mockScheduleCRUDStore {
	return &mockScheduleCRUDStore{
		schedules: make(map[string]*store.CronSchedule),
	}
}

func (m *mockScheduleCRUDStore) CreateSchedule(_ context.Context, p store.CreateScheduleParams) (*store.CronSchedule, error) {
	if m.err != nil {
		return nil, m.err
	}
	tz := p.Timezone
	if tz == "" {
		tz = "UTC"
	}
	sc := &store.CronSchedule{
		ScheduleID: uuid.New().String(),
		JobID:      p.JobID,
		CronExpr:   p.CronExpr,
		Timezone:   tz,
		Enabled:    true,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	m.schedules[sc.ScheduleID] = sc
	return sc, nil
}

func (m *mockScheduleCRUDStore) GetSchedule(_ context.Context, scheduleID string) (*store.CronSchedule, error) {
	if m.err != nil {
		return nil, m.err
	}
	sc, ok := m.schedules[scheduleID]
	if !ok {
		return nil, fmt.Errorf("schedule %s: %w", scheduleID, store.ErrNotFound)
	}
	cp := *sc
	return &cp, nil
}

func (m *mockScheduleCRUDStore) ListSchedulesByJob(_ context.Context, jobID string) ([]store.CronSchedule, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []store.CronSchedule
	for _, sc := range m.schedules {
		if sc.JobID == jobID {
			result = append(result, *sc)
		}
	}
	return result, nil
}

func (m *mockScheduleCRUDStore) UpdateSchedule(_ context.Context, p store.UpdateScheduleParams) (*store.CronSchedule, error) {
	if m.err != nil {
		return nil, m.err
	}
	sc, ok := m.schedules[p.ScheduleID]
	if !ok {
		return nil, fmt.Errorf("schedule %s: %w", p.ScheduleID, store.ErrNotFound)
	}
	sc.CronExpr = p.CronExpr
	sc.Timezone = p.Timezone
	sc.UpdatedAt = time.Now().UTC()
	cp := *sc
	return &cp, nil
}

func (m *mockScheduleCRUDStore) SetScheduleEnabled(_ context.Context, scheduleID string, enabled bool) error {
	if m.err != nil {
		return m.err
	}
	sc, ok := m.schedules[scheduleID]
	if !ok {
		return fmt.Errorf("schedule %s: %w", scheduleID, store.ErrNotFound)
	}
	sc.Enabled = enabled
	sc.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *mockScheduleCRUDStore) DeleteSchedule(_ context.Context, scheduleID string) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.schedules[scheduleID]; !ok {
		return fmt.Errorf("schedule %s: %w", scheduleID, store.ErrNotFound)
	}
	delete(m.schedules, scheduleID)
	return nil
}

// mockJobStoreForSchedules is a minimal mock implementing store.JobStoreIface.
type mockJobStoreForSchedules struct {
	jobs map[string]*store.JobDetail
	err  error
}

func newMockJobStoreForSchedules() *mockJobStoreForSchedules {
	return &mockJobStoreForSchedules{
		jobs: make(map[string]*store.JobDetail),
	}
}

func (m *mockJobStoreForSchedules) GetJob(_ context.Context, jobID string) (*store.JobDetail, error) {
	if m.err != nil {
		return nil, m.err
	}
	jd, ok := m.jobs[jobID]
	if !ok {
		return nil, fmt.Errorf("job %s: %w", jobID, store.ErrNotFound)
	}
	return jd, nil
}

func (m *mockJobStoreForSchedules) CreateJob(_ context.Context, p store.CreateJobParams) (*store.Job, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) ListJobs(_ context.Context) ([]store.Job, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) ListJobsByUser(_ context.Context, userID string) ([]store.Job, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) DeleteJob(_ context.Context, jobID string) error {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) UpdateJob(_ context.Context, p store.UpdateJobParams) (*store.Job, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) CreateStep(_ context.Context, p store.CreateStepParams) (*store.Step, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) UpdateStep(_ context.Context, p store.UpdateStepParams) error {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) DeleteStep(_ context.Context, stepID string) error {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) AddDependency(_ context.Context, stepID, dependsOn string) error {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) RemoveDependency(_ context.Context, stepID, dependsOn string) error {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) GetStepsWithDeps(_ context.Context, jobID string) ([]store.Step, []store.StepDependency, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) CreateRun(_ context.Context, p store.CreateRunParams) (*store.Run, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) InsertRunSteps(_ context.Context, runID string, steps []store.Step) error {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) GetRunWithSteps(_ context.Context, runID string) (*store.RunDetail, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) UpdateRunStepStatus(_ context.Context, runStepID, status, sessionID string, exitCode int) error {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) UpdateRunStatus(_ context.Context, runID, status string) error {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) ListRuns(_ context.Context, jobID string) ([]store.Run, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) ListAllRuns(_ context.Context, opts store.ListRunsOptions) ([]store.RunWithJobName, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) ListJobsWithStats(_ context.Context, userID string) ([]store.JobWithStats, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) UpdateRunParameters(_ context.Context, runID, parametersJSON string) error {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) UpdateRunStepAttempt(_ context.Context, runStepID string, attempt int) error {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) SetTaskValue(_ context.Context, runStepID, key, value string) error {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) GetTaskValues(_ context.Context, runStepID string) ([]store.TaskValue, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) GetTaskValuesForSteps(_ context.Context, runStepIDs []string) ([]store.TaskValue, error) {
	panic("not implemented")
}
func (m *mockJobStoreForSchedules) DeleteTaskValuesForStep(_ context.Context, runStepID string) error {
	panic("not implemented")
}

// mockScheduleReloader records calls.
type mockScheduleReloader struct {
	reloaded []string
	removed  []string
	reloadErr error
}

func (m *mockScheduleReloader) ReloadSchedule(_ context.Context, scheduleID string) error {
	m.reloaded = append(m.reloaded, scheduleID)
	return m.reloadErr
}

func (m *mockScheduleReloader) RemoveSchedule(scheduleID string) {
	m.removed = append(m.removed, scheduleID)
}

// --- router helper ---

type scheduleTestFixture struct {
	schedStore *mockScheduleCRUDStore
	jobStore   *mockJobStoreForSchedules
	reloader   *mockScheduleReloader
	srv        *httptest.Server
}

func newScheduleFixture() *scheduleTestFixture {
	schedStore := newMockScheduleCRUDStore()
	jobStore := newMockJobStoreForSchedules()
	reloader := &mockScheduleReloader{}

	h := handler.NewScheduleHandler(schedStore, jobStore, reloader, nil)

	r := chi.NewRouter()
	handler.RegisterScheduleRoutes(r, h)

	return &scheduleTestFixture{
		schedStore: schedStore,
		jobStore:   jobStore,
		reloader:   reloader,
		srv:        httptest.NewServer(r),
	}
}

func (f *scheduleTestFixture) close() {
	f.srv.Close()
}

// addJob adds a job to the mock job store.
func (f *scheduleTestFixture) addJob(jobID string) {
	f.jobStore.jobs[jobID] = &store.JobDetail{
		Job: store.Job{
			JobID:     jobID,
			Name:      "Test Job",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
}

// addSchedule adds a schedule directly to the mock store.
func (f *scheduleTestFixture) addSchedule(scheduleID, jobID, cronExpr string) {
	f.schedStore.schedules[scheduleID] = &store.CronSchedule{
		ScheduleID: scheduleID,
		JobID:      jobID,
		CronExpr:   cronExpr,
		Timezone:   "UTC",
		Enabled:    true,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
}

// --- List tests ---

func TestScheduleHandler_ListSchedules_Empty(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")

	resp, err := http.Get(fix.srv.URL + "/api/v1/jobs/job-1/schedules")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []store.CronSchedule
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 schedules, got %d", len(result))
	}
}

func TestScheduleHandler_ListSchedules_ReturnsSchedules(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")
	fix.addSchedule("s1", "job-1", "0 * * * *")
	fix.addSchedule("s2", "job-1", "0 0 * * *")
	fix.addSchedule("s3", "job-2", "0 0 * * *") // different job

	resp, err := http.Get(fix.srv.URL + "/api/v1/jobs/job-1/schedules")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []store.CronSchedule
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 schedules for job-1, got %d", len(result))
	}
}

func TestScheduleHandler_ListSchedules_JobNotFound(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	resp, err := http.Get(fix.srv.URL + "/api/v1/jobs/ghost/schedules")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// --- Create tests ---

func TestScheduleHandler_CreateSchedule(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")

	body := map[string]interface{}{
		"cron_expr": "0 * * * *",
		"timezone":  "America/New_York",
	}
	b, _ := json.Marshal(body)
	resp, err := http.Post(fix.srv.URL+"/api/v1/jobs/job-1/schedules", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created store.CronSchedule
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ScheduleID == "" {
		t.Error("expected non-empty schedule_id")
	}
	if created.JobID != "job-1" {
		t.Errorf("JobID = %q, want %q", created.JobID, "job-1")
	}
	if created.CronExpr != "0 * * * *" {
		t.Errorf("CronExpr = %q, want %q", created.CronExpr, "0 * * * *")
	}
	if created.Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want %q", created.Timezone, "America/New_York")
	}
	if !created.Enabled {
		t.Error("expected Enabled = true")
	}
	if len(fix.reloader.reloaded) != 1 {
		t.Errorf("expected 1 reload call, got %d", len(fix.reloader.reloaded))
	}
}

func TestScheduleHandler_CreateSchedule_MissingCronExpr(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")

	body := map[string]interface{}{"timezone": "UTC"}
	b, _ := json.Marshal(body)
	resp, err := http.Post(fix.srv.URL+"/api/v1/jobs/job-1/schedules", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestScheduleHandler_CreateSchedule_InvalidCronExpr(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")

	body := map[string]interface{}{"cron_expr": "not-a-cron"}
	b, _ := json.Marshal(body)
	resp, err := http.Post(fix.srv.URL+"/api/v1/jobs/job-1/schedules", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestScheduleHandler_CreateSchedule_InvalidTimezone(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")

	body := map[string]interface{}{
		"cron_expr": "0 * * * *",
		"timezone":  "Not/A/Timezone",
	}
	b, _ := json.Marshal(body)
	resp, err := http.Post(fix.srv.URL+"/api/v1/jobs/job-1/schedules", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestScheduleHandler_CreateSchedule_JobNotFound(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	body := map[string]interface{}{"cron_expr": "0 * * * *"}
	b, _ := json.Marshal(body)
	resp, err := http.Post(fix.srv.URL+"/api/v1/jobs/ghost/schedules", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestScheduleHandler_CreateSchedule_InvalidBody(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")

	resp, err := http.Post(fix.srv.URL+"/api/v1/jobs/job-1/schedules", "application/json", bytes.NewBufferString("not-json"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// --- Get tests ---

func TestScheduleHandler_GetSchedule(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")
	fix.addSchedule("sched-1", "job-1", "0 * * * *")

	resp, err := http.Get(fix.srv.URL + "/api/v1/schedules/sched-1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var sc store.CronSchedule
	if err := json.NewDecoder(resp.Body).Decode(&sc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sc.ScheduleID != "sched-1" {
		t.Errorf("ScheduleID = %q, want %q", sc.ScheduleID, "sched-1")
	}
}

func TestScheduleHandler_GetSchedule_NotFound(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	resp, err := http.Get(fix.srv.URL + "/api/v1/schedules/ghost")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// --- Update tests ---

func TestScheduleHandler_UpdateSchedule(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")
	fix.addSchedule("sched-1", "job-1", "0 * * * *")

	body := map[string]interface{}{
		"cron_expr": "0 0 * * *",
		"timezone":  "UTC",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, fix.srv.URL+"/api/v1/schedules/sched-1", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var updated store.CronSchedule
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.CronExpr != "0 0 * * *" {
		t.Errorf("CronExpr = %q, want %q", updated.CronExpr, "0 0 * * *")
	}
	if len(fix.reloader.reloaded) != 1 {
		t.Errorf("expected 1 reload call, got %d", len(fix.reloader.reloaded))
	}
}

func TestScheduleHandler_UpdateSchedule_NotFound(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	body := map[string]interface{}{"cron_expr": "0 0 * * *"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, fix.srv.URL+"/api/v1/schedules/ghost", bytes.NewBuffer(b))
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

func TestScheduleHandler_UpdateSchedule_InvalidCronExpr(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")
	fix.addSchedule("sched-1", "job-1", "0 * * * *")

	body := map[string]interface{}{"cron_expr": "bad-expr"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, fix.srv.URL+"/api/v1/schedules/sched-1", bytes.NewBuffer(b))
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

// --- Delete tests ---

func TestScheduleHandler_DeleteSchedule(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")
	schedID := uuid.New().String()
	fix.addSchedule(schedID, "job-1", "0 * * * *")

	req, _ := http.NewRequest(http.MethodDelete, fix.srv.URL+"/api/v1/schedules/"+schedID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
	if _, ok := fix.schedStore.schedules[schedID]; ok {
		t.Error("schedule still exists after delete")
	}
	if len(fix.reloader.removed) != 1 || fix.reloader.removed[0] != schedID {
		t.Errorf("expected RemoveSchedule called with %q, got %v", schedID, fix.reloader.removed)
	}
}

func TestScheduleHandler_DeleteSchedule_NotFound(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	req, _ := http.NewRequest(http.MethodDelete, fix.srv.URL+"/api/v1/schedules/ghost", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// --- Pause tests ---

func TestScheduleHandler_PauseSchedule(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")
	fix.addSchedule("sched-1", "job-1", "0 * * * *")

	resp, err := http.Post(fix.srv.URL+"/api/v1/schedules/sched-1/pause", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var sc store.CronSchedule
	if err := json.NewDecoder(resp.Body).Decode(&sc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sc.Enabled {
		t.Error("expected Enabled = false after pause")
	}
	if len(fix.reloader.reloaded) != 1 {
		t.Errorf("expected 1 reload call, got %d", len(fix.reloader.reloaded))
	}
}

func TestScheduleHandler_PauseSchedule_NotFound(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	resp, err := http.Post(fix.srv.URL+"/api/v1/schedules/ghost/pause", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// --- Resume tests ---

func TestScheduleHandler_ResumeSchedule(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.addJob("job-1")
	fix.addSchedule("sched-1", "job-1", "0 * * * *")
	fix.schedStore.schedules["sched-1"].Enabled = false

	resp, err := http.Post(fix.srv.URL+"/api/v1/schedules/sched-1/resume", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var sc store.CronSchedule
	if err := json.NewDecoder(resp.Body).Decode(&sc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !sc.Enabled {
		t.Error("expected Enabled = true after resume")
	}
	if len(fix.reloader.reloaded) != 1 {
		t.Errorf("expected 1 reload call, got %d", len(fix.reloader.reloaded))
	}
}

func TestScheduleHandler_ResumeSchedule_NotFound(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	resp, err := http.Post(fix.srv.URL+"/api/v1/schedules/ghost/resume", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// --- ErrNotFound wrapping check for GetSchedule ---

func TestScheduleHandler_GetSchedule_StoreErrNotFound(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	// Override mock to return store.ErrNotFound explicitly.
	fix.schedStore.err = store.ErrNotFound

	resp, err := http.Get(fix.srv.URL + "/api/v1/schedules/any-id")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestScheduleHandler_DeleteSchedule_StoreError(t *testing.T) {
	fix := newScheduleFixture()
	defer fix.close()

	fix.schedStore.err = context.DeadlineExceeded

	req, _ := http.NewRequest(http.MethodDelete, fix.srv.URL+"/api/v1/schedules/any-id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}
