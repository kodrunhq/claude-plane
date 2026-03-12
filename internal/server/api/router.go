package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"

	"github.com/claudeplane/claude-plane/internal/server/auth"
	"github.com/claudeplane/claude-plane/internal/server/connmgr"
	"github.com/claudeplane/claude-plane/internal/server/handler"
	"github.com/claudeplane/claude-plane/internal/server/session"
	"github.com/claudeplane/claude-plane/internal/server/store"
)

// Handlers holds the dependencies required by all HTTP handlers.
type Handlers struct {
	store   *store.Store
	authSvc *auth.Service
	connMgr *connmgr.ConnectionManager
}

// NewHandlers creates a new Handlers instance with the given dependencies.
func NewHandlers(s *store.Store, authSvc *auth.Service, connMgr *connmgr.ConnectionManager) *Handlers {
	return &Handlers{
		store:   s,
		authSvc: authSvc,
		connMgr: connMgr,
	}
}

// maxBytesMiddleware limits the size of request bodies.
func maxBytesMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NewRouter creates a chi router with all API routes configured.
// Public routes (register, login) require no authentication.
// Protected routes (logout, machines, sessions) require a valid JWT Bearer token.
// The WebSocket route uses query-param authentication (WebSocket can't send headers).
func NewRouter(h *Handlers, sessionHandler *session.SessionHandler, wsHandler http.HandlerFunc, jobHandler *handler.JobHandler, runHandler *handler.RunHandler) chi.Router {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(maxBytesMiddleware(1 << 20)) // 1MB global limit
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(securityHeadersMiddleware)

	// WebSocket route — uses query param auth, not JWT middleware
	if wsHandler != nil {
		r.Get("/ws/terminal/{sessionID}", wsHandler)
	}

	// 5 requests per minute per IP for auth endpoints
	authLimiter := RateLimitMiddleware(rate.Limit(5.0/60.0), 5)

	// Public routes
	r.Route("/api/v1", func(r chi.Router) {
		r.With(authLimiter).Post("/auth/register", h.Register)
		r.With(authLimiter).Post("/auth/login", h.Login)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(JWTAuthMiddleware(h.authSvc))
			r.Post("/auth/logout", h.Logout)
			r.Get("/machines", h.ListMachines)
			r.Get("/machines/{machineID}", h.GetMachine)

			// Session routes
			if sessionHandler != nil {
				r.Post("/sessions", sessionHandler.CreateSession)
				r.Get("/sessions", sessionHandler.ListSessions)
				r.Get("/sessions/{sessionID}", sessionHandler.GetSession)
				r.Delete("/sessions/{sessionID}", sessionHandler.TerminateSession)
			}
		})
	})

	// Job system routes (flat paths, JWT-protected)
	if jobHandler != nil || runHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(JWTAuthMiddleware(h.authSvc))
			if jobHandler != nil {
				handler.RegisterJobRoutes(r, jobHandler)
			}
			if runHandler != nil {
				handler.RegisterRunRoutes(r, runHandler)
			}
		})
	}

	return r
}
