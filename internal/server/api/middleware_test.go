package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/api"
	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// --- fakes ---

// fakeAPIKeyStore satisfies api.APIKeyStore for unit tests.
type fakeAPIKeyStore struct {
	mu             sync.Mutex
	keys           map[string]*store.APIKey // plaintextKey -> APIKey
	lastUsedCalled []string
	validateErr    error
	updateErr      error
}

func newFakeAPIKeyStore() *fakeAPIKeyStore {
	return &fakeAPIKeyStore{keys: make(map[string]*store.APIKey)}
}

func (f *fakeAPIKeyStore) addKey(plaintext string, key *store.APIKey) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys[plaintext] = key
}

func (f *fakeAPIKeyStore) ValidateAPIKey(_ context.Context, plaintextKey string, _ []byte) (*store.APIKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.validateErr != nil {
		return nil, f.validateErr
	}
	k, ok := f.keys[plaintextKey]
	if !ok {
		return nil, store.ErrNotFound
	}
	return k, nil
}

func (f *fakeAPIKeyStore) UpdateAPIKeyLastUsed(_ context.Context, keyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastUsedCalled = append(f.lastUsedCalled, keyID)
	return f.updateErr
}

func (f *fakeAPIKeyStore) lastUsedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(f.lastUsedCalled))
	copy(cp, f.lastUsedCalled)
	return cp
}

// fakeUserStore satisfies api.UserStore for unit tests.
type fakeUserStore struct {
	users   map[string]*store.User
	userErr error
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{users: make(map[string]*store.User)}
}

func (f *fakeUserStore) addUser(u *store.User) {
	f.users[u.UserID] = u
}

func (f *fakeUserStore) GetUserByID(_ context.Context, userID string) (*store.User, error) {
	if f.userErr != nil {
		return nil, f.userErr
	}
	u, ok := f.users[userID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return u, nil
}

// --- helpers ---

// echoClaimsHandler writes 200 with the user ID from claims.
func echoClaimsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := api.GetClaims(r)
		if claims == nil {
			http.Error(w, "no claims", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(claims.UserID + ":" + claims.Role))
	}
}

func buildAuthSvc(t *testing.T) *auth.Service {
	t.Helper()
	return auth.NewService([]byte("test-signing-key-32-bytes-long!!"), 15*time.Minute, nil)
}

// --- tests ---

// TestJWTAuthMiddleware_APIKeyValid confirms a cpk_ token is validated via
// the API key store and results in claims being set on the context.
func TestJWTAuthMiddleware_APIKeyValid(t *testing.T) {
	authSvc := buildAuthSvc(t)
	keyStore := newFakeAPIKeyStore()
	userStore := newFakeUserStore()

	const plaintext = "cpk_abcd1234_somerandombits"
	const userID = "user-001"
	const role = "admin"

	keyStore.addKey(plaintext, &store.APIKey{KeyID: "key-001", UserID: userID})
	userStore.addUser(&store.User{UserID: userID, Email: "alice@example.com", Role: role})

	aka := &api.APIKeyAuth{
		Store:      keyStore,
		UserStore:  userStore,
		SigningKey: []byte("signing-key"),
	}

	mw := api.JWTAuthMiddleware(authSvc, aka)
	handler := mw(echoClaimsHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	got := rec.Body.String()
	if got != userID+":"+role {
		t.Errorf("expected body %q, got %q", userID+":"+role, got)
	}
}

// TestJWTAuthMiddleware_JWTValid confirms a normal JWT token still works when
// APIKeyAuth is provided (existing flow must be unaffected).
func TestJWTAuthMiddleware_JWTValid(t *testing.T) {
	authSvc := buildAuthSvc(t)
	user := &store.User{UserID: "user-jwt", Email: "bob@example.com", Role: "user"}
	token, err := authSvc.IssueToken(user)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	keyStore := newFakeAPIKeyStore()
	userStore := newFakeUserStore()
	aka := &api.APIKeyAuth{Store: keyStore, UserStore: userStore, SigningKey: []byte("k")}

	mw := api.JWTAuthMiddleware(authSvc, aka)
	handler := mw(echoClaimsHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestJWTAuthMiddleware_JWTValidNoAPIKeyAuth confirms the middleware keeps
// working for JWT tokens when no APIKeyAuth is provided (original call site).
func TestJWTAuthMiddleware_JWTValidNoAPIKeyAuth(t *testing.T) {
	authSvc := buildAuthSvc(t)
	user := &store.User{UserID: "user-jwt-bare", Email: "c@example.com", Role: "user"}
	token, err := authSvc.IssueToken(user)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	mw := api.JWTAuthMiddleware(authSvc) // no APIKeyAuth
	handler := mw(echoClaimsHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// TestJWTAuthMiddleware_APIKeyInvalid confirms that a cpk_ token that fails
// store validation returns 401.
func TestJWTAuthMiddleware_APIKeyInvalid(t *testing.T) {
	authSvc := buildAuthSvc(t)
	keyStore := newFakeAPIKeyStore()
	userStore := newFakeUserStore()

	// Do NOT add the key to the store — validation must fail.
	aka := &api.APIKeyAuth{Store: keyStore, UserStore: userStore, SigningKey: []byte("k")}

	mw := api.JWTAuthMiddleware(authSvc, aka)
	handler := mw(echoClaimsHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer cpk_deadbeef_doesnotexist")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// TestJWTAuthMiddleware_APIKeyExpired confirms that the store returning an
// expiry error results in 401.
func TestJWTAuthMiddleware_APIKeyExpired(t *testing.T) {
	authSvc := buildAuthSvc(t)
	keyStore := newFakeAPIKeyStore()
	keyStore.validateErr = store.ErrNotFound // store returns error for expired/unknown key
	userStore := newFakeUserStore()

	aka := &api.APIKeyAuth{Store: keyStore, UserStore: userStore, SigningKey: []byte("k")}
	mw := api.JWTAuthMiddleware(authSvc, aka)
	handler := mw(echoClaimsHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer cpk_abcd1234_expiredkey")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// TestJWTAuthMiddleware_APIKeyNoDependencies confirms that when a cpk_ token
// is presented but no APIKeyAuth was configured, the middleware returns 401.
func TestJWTAuthMiddleware_APIKeyNoDependencies(t *testing.T) {
	authSvc := buildAuthSvc(t)
	mw := api.JWTAuthMiddleware(authSvc) // no APIKeyAuth
	handler := mw(echoClaimsHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer cpk_abcd1234_something")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// TestJWTAuthMiddleware_LastUsedUpdatedAsync confirms that UpdateAPIKeyLastUsed
// is called asynchronously after successful API key authentication.
func TestJWTAuthMiddleware_LastUsedUpdatedAsync(t *testing.T) {
	authSvc := buildAuthSvc(t)
	keyStore := newFakeAPIKeyStore()
	userStore := newFakeUserStore()

	const plaintext = "cpk_aabbccdd_randombits"
	const keyID = "key-async-001"
	const userID = "user-async-001"

	keyStore.addKey(plaintext, &store.APIKey{KeyID: keyID, UserID: userID})
	userStore.addUser(&store.User{UserID: userID, Email: "d@example.com", Role: "user"})

	aka := &api.APIKeyAuth{
		Store:      keyStore,
		UserStore:  userStore,
		SigningKey: []byte("k"),
	}

	mw := api.JWTAuthMiddleware(authSvc, aka)
	handler := mw(echoClaimsHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Give the goroutine time to call UpdateAPIKeyLastUsed.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		ids := keyStore.lastUsedIDs()
		if len(ids) > 0 {
			if ids[0] != keyID {
				t.Errorf("UpdateAPIKeyLastUsed called with %q, want %q", ids[0], keyID)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("UpdateAPIKeyLastUsed was not called within 500ms")
}

// TestJWTAuthMiddleware_MissingAuth confirms 401 when no credentials are provided.
func TestJWTAuthMiddleware_MissingAuth(t *testing.T) {
	authSvc := buildAuthSvc(t)
	mw := api.JWTAuthMiddleware(authSvc)
	handler := mw(echoClaimsHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// TestJWTAuthMiddleware_InvalidBearerFormat confirms 401 for malformed header.
func TestJWTAuthMiddleware_InvalidBearerFormat(t *testing.T) {
	authSvc := buildAuthSvc(t)
	mw := api.JWTAuthMiddleware(authSvc)
	handler := mw(echoClaimsHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Token something")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// TestJWTAuthMiddleware_APIKeyUserNotFound confirms 401 when the API key is
// valid but the associated user cannot be found in the user store.
func TestJWTAuthMiddleware_APIKeyUserNotFound(t *testing.T) {
	authSvc := buildAuthSvc(t)
	keyStore := newFakeAPIKeyStore()
	userStore := newFakeUserStore()
	userStore.userErr = store.ErrNotFound

	const plaintext = "cpk_aabbccdd_randombits2"
	keyStore.addKey(plaintext, &store.APIKey{KeyID: "key-002", UserID: "ghost-user"})
	// Do NOT add the user to the store.

	aka := &api.APIKeyAuth{Store: keyStore, UserStore: userStore, SigningKey: []byte("k")}
	mw := api.JWTAuthMiddleware(authSvc, aka)
	handler := mw(echoClaimsHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
