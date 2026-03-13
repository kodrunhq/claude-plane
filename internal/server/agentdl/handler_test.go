package agentdl_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/agentdl"
)

// newTestRouter builds a chi router with the agentdl routes mounted, using
// the provided in-memory filesystem as the binary source.
func newTestRouter(binariesFS fstest.MapFS) chi.Router {
	h := agentdl.NewHandler(binariesFS)
	r := chi.NewRouter()
	agentdl.RegisterRoutes(r, h)
	return r
}

func TestServeDownload_InvalidPlatform(t *testing.T) {
	router := newTestRouter(fstest.MapFS{})

	cases := []string{"windows-amd64", "linux-386", "bad", "", "../../etc/passwd"}
	for _, platform := range cases {
		t.Run(platform, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/dl/agent/"+platform, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			// chi returns 404 for routes that don't match the pattern; the
			// handler itself returns 400 for matched-but-invalid values.
			// Either way the binary must not be served.
			if rec.Code == http.StatusOK {
				t.Errorf("expected non-200 for platform %q, got 200", platform)
			}
		})
	}
}

func TestServeDownload_ValidPlatform_BinaryMissing(t *testing.T) {
	// Empty FS — no binaries present.
	router := newTestRouter(fstest.MapFS{})

	platforms := []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"}
	for _, platform := range platforms {
		t.Run(platform, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/dl/agent/"+platform, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("expected 404 for missing binary %q, got %d", platform, rec.Code)
			}
		})
	}
}

func TestServeDownload_ValidPlatform_BinaryPresent(t *testing.T) {
	const platform = "linux-amd64"
	const binaryContent = "fake-elf-binary"

	testFS := fstest.MapFS{
		"binaries/claude-plane-agent-" + platform: {
			Data: []byte(binaryContent),
		},
	}
	router := newTestRouter(testFS)

	req := httptest.NewRequest(http.MethodGet, "/dl/agent/"+platform, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	gotCT := rec.Header().Get("Content-Type")
	if gotCT != "application/octet-stream" {
		t.Errorf("Content-Type: got %q, want %q", gotCT, "application/octet-stream")
	}

	gotCD := rec.Header().Get("Content-Disposition")
	wantCD := "attachment; filename=claude-plane-agent"
	if gotCD != wantCD {
		t.Errorf("Content-Disposition: got %q, want %q", gotCD, wantCD)
	}

	if body := rec.Body.String(); body != binaryContent {
		t.Errorf("body: got %q, want %q", body, binaryContent)
	}
}

func TestServeDownload_AllValidPlatforms_HeadersCorrect(t *testing.T) {
	platforms := []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"}

	// Pre-populate the FS with a stub binary for every valid platform.
	testFS := make(fstest.MapFS)
	for _, p := range platforms {
		testFS["binaries/claude-plane-agent-"+p] = &fstest.MapFile{
			Data: []byte("stub-" + p),
		}
	}

	router := newTestRouter(testFS)

	for _, platform := range platforms {
		t.Run(platform, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/dl/agent/"+platform, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 for %q, got %d", platform, rec.Code)
			}

			if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
				t.Errorf("Content-Type: got %q, want application/octet-stream", ct)
			}

			if cd := rec.Header().Get("Content-Disposition"); cd != "attachment; filename=claude-plane-agent" {
				t.Errorf("Content-Disposition: got %q", cd)
			}
		})
	}
}
