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

// claimsMiddleware injects a ClaimsGetter that returns fixed claims.
func claimsMiddleware(userID, role string) handler.ClaimsGetter {
	return func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: userID, Role: role}
	}
}

// seedUser creates a user in the store for foreign key satisfaction.
func seedUser(t *testing.T, s *store.Store, userID, role string) {
	t.Helper()
	hash, err := store.HashPassword("testpass")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// Ignore duplicate errors for convenience.
	_ = s.CreateUser(&store.User{
		UserID:       userID,
		Email:        userID + "@test.com",
		DisplayName:  "Test " + userID,
		PasswordHash: hash,
		Role:         role,
	})
}

// newTemplateRouter creates a chi router with template routes and a fixed user.
// It also seeds the user in the database to satisfy foreign key constraints.
func newTemplateRouter(t *testing.T, s *store.Store, userID, role string) *httptest.Server {
	t.Helper()
	seedUser(t, s, userID, role)
	h := handler.NewTemplateHandler(s, claimsMiddleware(userID, role))
	r := chi.NewRouter()
	handler.RegisterTemplateRoutes(r, h)
	return httptest.NewServer(r)
}

func TestTemplateHandler_Create_Valid(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"name":           "My Template",
		"description":    "A test template",
		"initial_prompt": "Hello ${WORLD}",
	})
	resp, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var result store.SessionTemplate
	json.NewDecoder(resp.Body).Decode(&result)
	if result.TemplateID == "" {
		t.Error("expected template_id in response")
	}
	if result.Name != "My Template" {
		t.Errorf("expected name 'My Template', got %q", result.Name)
	}
}

func TestTemplateHandler_Create_MissingName(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"description": "No name",
	})
	resp, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTemplateHandler_Create_DuplicateName(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{"name": "Duplicate"})

	resp1, _ := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", resp1.StatusCode)
	}

	body2, _ := json.Marshal(map[string]interface{}{"name": "Duplicate"})
	resp2, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body2))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp2.StatusCode)
	}
}

func TestTemplateHandler_Create_InvalidVariable(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"name":           "Bad Var",
		"initial_prompt": "Hello ${lowercase}",
	})
	resp, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTemplateHandler_List_UserTemplates(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	for _, name := range []string{"Tmpl A", "Tmpl B"} {
		body, _ := json.Marshal(map[string]interface{}{"name": name})
		resp, _ := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		resp.Body.Close()
	}

	resp, err := http.Get(srv.URL + "/api/v1/templates")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []store.SessionTemplate
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) != 2 {
		t.Errorf("expected 2 templates, got %d", len(result))
	}
}

func TestTemplateHandler_List_AdminSeesAll(t *testing.T) {
	s := newTestStore(t)

	// Create templates as user-1
	srv1 := newTemplateRouter(t, s, "user-1", "member")
	body1, _ := json.Marshal(map[string]interface{}{"name": "User1 Tmpl"})
	resp1, _ := http.Post(srv1.URL+"/api/v1/templates", "application/json", bytes.NewReader(body1))
	resp1.Body.Close()
	srv1.Close()

	// Create templates as user-2
	srv2 := newTemplateRouter(t, s, "user-2", "member")
	body2, _ := json.Marshal(map[string]interface{}{"name": "User2 Tmpl"})
	resp2, _ := http.Post(srv2.URL+"/api/v1/templates", "application/json", bytes.NewReader(body2))
	resp2.Body.Close()
	srv2.Close()

	// Admin lists all
	srvAdmin := newTemplateRouter(t, s, "admin-1", "admin")
	defer srvAdmin.Close()

	resp, err := http.Get(srvAdmin.URL + "/api/v1/templates")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var result []store.SessionTemplate
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) != 2 {
		t.Errorf("admin expected 2 templates, got %d", len(result))
	}
}

func TestTemplateHandler_List_TagFilter(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	body1, _ := json.Marshal(map[string]interface{}{"name": "Tagged", "tags": []string{"dev"}})
	r1, _ := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body1))
	r1.Body.Close()

	body2, _ := json.Marshal(map[string]interface{}{"name": "Untagged"})
	r2, _ := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body2))
	r2.Body.Close()

	resp, err := http.Get(srv.URL + "/api/v1/templates?tag=dev")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var result []store.SessionTemplate
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) != 1 {
		t.Errorf("expected 1 tagged template, got %d", len(result))
	}
	if len(result) == 1 && result[0].Name != "Tagged" {
		t.Errorf("expected 'Tagged', got %q", result[0].Name)
	}
}

func TestTemplateHandler_List_NameFilter(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	for _, name := range []string{"Alpha", "Beta"} {
		body, _ := json.Marshal(map[string]interface{}{"name": name})
		r, _ := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		r.Body.Close()
	}

	resp, err := http.Get(srv.URL + "/api/v1/templates?name=Alpha")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var result []store.SessionTemplate
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) != 1 {
		t.Errorf("expected 1 template, got %d", len(result))
	}
}

func TestTemplateHandler_Get_Exists(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{"name": "Get Test"})
	createResp, _ := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	var created store.SessionTemplate
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	resp, err := http.Get(srv.URL + "/api/v1/templates/" + created.TemplateID)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result store.SessionTemplate
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Name != "Get Test" {
		t.Errorf("expected 'Get Test', got %q", result.Name)
	}
}

func TestTemplateHandler_Get_NotFound(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/templates/nonexistent-id")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestTemplateHandler_Get_OtherUser(t *testing.T) {
	s := newTestStore(t)

	// Create as user-1
	srv1 := newTemplateRouter(t, s, "user-1", "member")
	body, _ := json.Marshal(map[string]interface{}{"name": "Private"})
	createResp, _ := http.Post(srv1.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	var created store.SessionTemplate
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()
	srv1.Close()

	// Try to get as user-2
	srv2 := newTemplateRouter(t, s, "user-2", "member")
	defer srv2.Close()

	resp, err := http.Get(srv2.URL + "/api/v1/templates/" + created.TemplateID)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for other user, got %d", resp.StatusCode)
	}
}

func TestTemplateHandler_Get_AdminSeesAll(t *testing.T) {
	s := newTestStore(t)

	// Create as user-1
	srv1 := newTemplateRouter(t, s, "user-1", "member")
	body, _ := json.Marshal(map[string]interface{}{"name": "User1 Private"})
	createResp, _ := http.Post(srv1.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	var created store.SessionTemplate
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()
	srv1.Close()

	// Admin can see it
	srvAdmin := newTemplateRouter(t, s, "admin-1", "admin")
	defer srvAdmin.Close()

	resp, err := http.Get(srvAdmin.URL + "/api/v1/templates/" + created.TemplateID)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", resp.StatusCode)
	}
}

func TestTemplateHandler_Update_Valid(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{"name": "Original"})
	createResp, _ := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	var created store.SessionTemplate
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	updateBody, _ := json.Marshal(map[string]interface{}{"name": "Updated", "description": "new desc"})
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/templates/"+created.TemplateID, bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result store.SessionTemplate
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Name != "Updated" {
		t.Errorf("expected 'Updated', got %q", result.Name)
	}
	if result.Description != "new desc" {
		t.Errorf("expected 'new desc', got %q", result.Description)
	}
}

func TestTemplateHandler_Update_NonOwner(t *testing.T) {
	s := newTestStore(t)

	// Create as user-1
	srv1 := newTemplateRouter(t, s, "user-1", "member")
	body, _ := json.Marshal(map[string]interface{}{"name": "Owner Only"})
	createResp, _ := http.Post(srv1.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	var created store.SessionTemplate
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()
	srv1.Close()

	// Update as user-2
	srv2 := newTemplateRouter(t, s, "user-2", "member")
	defer srv2.Close()

	updateBody, _ := json.Marshal(map[string]interface{}{"name": "Hijacked"})
	req, _ := http.NewRequest("PUT", srv2.URL+"/api/v1/templates/"+created.TemplateID, bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for non-owner, got %d", resp.StatusCode)
	}
}

func TestTemplateHandler_Delete_Valid(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{"name": "Delete Me"})
	createResp, _ := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	var created store.SessionTemplate
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/templates/"+created.TemplateID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Verify it's gone
	getResp, err := http.Get(srv.URL + "/api/v1/templates/" + created.TemplateID)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}

func TestTemplateHandler_Delete_NonOwner(t *testing.T) {
	s := newTestStore(t)

	// Create as user-1
	srv1 := newTemplateRouter(t, s, "user-1", "member")
	body, _ := json.Marshal(map[string]interface{}{"name": "Not Yours"})
	createResp, _ := http.Post(srv1.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	var created store.SessionTemplate
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()
	srv1.Close()

	// Delete as user-2
	srv2 := newTemplateRouter(t, s, "user-2", "member")
	defer srv2.Close()

	req, _ := http.NewRequest("DELETE", srv2.URL+"/api/v1/templates/"+created.TemplateID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for non-owner, got %d", resp.StatusCode)
	}
}

func TestTemplateHandler_Clone_Valid(t *testing.T) {
	s := newTestStore(t)
	srv := newTemplateRouter(t, s, "user-1", "member")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"name":        "Clone Source",
		"description": "original desc",
		"tags":        []string{"prod"},
	})
	createResp, _ := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
	var created store.SessionTemplate
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	resp, err := http.Post(srv.URL+"/api/v1/templates/"+created.TemplateID+"/clone", "application/json", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var cloned store.SessionTemplate
	json.NewDecoder(resp.Body).Decode(&cloned)
	if cloned.TemplateID == created.TemplateID {
		t.Error("cloned template should have a different ID")
	}
	if cloned.Name != "Clone Source-copy" {
		t.Errorf("expected 'Clone Source-copy', got %q", cloned.Name)
	}
	if cloned.Description != "original desc" {
		t.Errorf("expected 'original desc', got %q", cloned.Description)
	}
}
