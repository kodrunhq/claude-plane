package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kodrunhq/claude-plane/internal/server/testutil"
)

// requireFields asserts that every key in required exists in the map.
func requireFields(t *testing.T, entity map[string]interface{}, required []string, label string) {
	t.Helper()
	for _, field := range required {
		if _, ok := entity[field]; !ok {
			t.Errorf("%s: missing required field %q", label, field)
		}
	}
}

// requireAbsentFields asserts that none of the keys in absent exist in the map.
func requireAbsentFields(t *testing.T, entity map[string]interface{}, absent []string, label string) {
	t.Helper()
	for _, field := range absent {
		if v, ok := entity[field]; ok && v != nil && v != "" {
			t.Errorf("%s: sensitive field %q should be absent or empty in list response, got %v", label, field, v)
		}
	}
}

// decodeJSONArray reads the response body and decodes it into a slice of maps.
func decodeJSONArray(t *testing.T, resp *http.Response) []map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

func TestContractMachinesList(t *testing.T) {
	ts := testutil.NewTestServer(t)
	cookies := ts.LoginAsAdmin(t)

	// Seed a machine.
	testutil.MustCreateMachine(t, ts.Store, testutil.WithMachineID("contract-m1"))

	resp := ts.AuthRequest(t, http.MethodGet, "/api/v1/machines", nil, cookies)
	machines := decodeJSONArray(t, resp)

	if len(machines) == 0 {
		t.Fatal("expected at least one machine")
	}

	required := []string{"machine_id", "status", "max_sessions", "created_at"}
	for i, m := range machines {
		requireFields(t, m, required, "machines["+itoa(i)+"]")
	}
}

func TestContractJobsList(t *testing.T) {
	ts := testutil.NewTestServer(t)
	cookies := ts.LoginAsAdmin(t)

	// Seed a job.
	testutil.MustCreateJob(t, ts.Store, testutil.WithJobName("contract-job"))

	resp := ts.AuthRequest(t, http.MethodGet, "/api/v1/jobs", nil, cookies)
	jobs := decodeJSONArray(t, resp)

	if len(jobs) == 0 {
		t.Fatal("expected at least one job")
	}

	required := []string{"job_id", "name", "created_at"}
	for i, j := range jobs {
		requireFields(t, j, required, "jobs["+itoa(i)+"]")
	}
}

func TestContractSessionsList(t *testing.T) {
	ts := testutil.NewTestServer(t)
	cookies := ts.LoginAsAdmin(t)

	// Seed a machine and session.
	machineID := testutil.MustCreateMachine(t, ts.Store, testutil.WithMachineID("contract-m2"))
	testutil.MustCreateSession(t, ts.Store, machineID,
		testutil.WithSessionEnvVars(`{"SECRET":"hunter2"}`),
		testutil.WithSessionArgs(`["--verbose"]`),
		testutil.WithSessionInitialPrompt("do something secret"),
	)

	resp := ts.AuthRequest(t, http.MethodGet, "/api/v1/sessions", nil, cookies)
	sessions := decodeJSONArray(t, resp)

	if len(sessions) == 0 {
		t.Fatal("expected at least one session")
	}

	required := []string{"session_id", "machine_id", "status", "command", "created_at"}
	sensitive := []string{"env_vars", "args", "initial_prompt"}

	for i, s := range sessions {
		label := "sessions[" + itoa(i) + "]"
		requireFields(t, s, required, label)
		requireAbsentFields(t, s, sensitive, label)
	}
}

func TestContractTemplatesList(t *testing.T) {
	ts := testutil.NewTestServer(t)
	cookies := ts.LoginAsAdmin(t)

	// Seed a template (needs a user_id; use the admin user from LoginAsAdmin).
	// Fetch admin user_id from the /auth/me endpoint.
	meResp := ts.AuthRequest(t, http.MethodGet, "/api/v1/auth/me", nil, cookies)
	defer meResp.Body.Close()
	var meData map[string]interface{}
	if err := json.NewDecoder(meResp.Body).Decode(&meData); err != nil {
		t.Fatalf("decode /auth/me: %v", err)
	}
	userID, _ := meData["user_id"].(string)
	if userID == "" {
		t.Fatal("expected user_id from /auth/me")
	}

	testutil.MustCreateTemplate(t, ts.Store, userID, testutil.WithTemplateName("contract-tmpl"))

	resp := ts.AuthRequest(t, http.MethodGet, "/api/v1/templates", nil, cookies)
	templates := decodeJSONArray(t, resp)

	if len(templates) == 0 {
		t.Fatal("expected at least one template")
	}

	required := []string{"template_id", "name", "user_id", "created_at"}
	for i, tmpl := range templates {
		requireFields(t, tmpl, required, "templates["+itoa(i)+"]")
	}
}

func TestContractUsersList(t *testing.T) {
	ts := testutil.NewTestServer(t)
	cookies := ts.LoginAsAdmin(t)

	// The admin user created by LoginAsAdmin is already in the store.
	resp := ts.AuthRequest(t, http.MethodGet, "/api/v1/users", nil, cookies)
	users := decodeJSONArray(t, resp)

	if len(users) == 0 {
		t.Fatal("expected at least one user")
	}

	required := []string{"user_id", "email", "role", "created_at"}
	for i, u := range users {
		requireFields(t, u, required, "users["+itoa(i)+"]")

		// password_hash must never appear in API responses
		if _, ok := u["password_hash"]; ok {
			t.Errorf("users[%d]: password_hash must not be exposed in API response", i)
		}
	}
}

// itoa converts an int to a string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
