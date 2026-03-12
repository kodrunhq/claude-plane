package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/claudeplane/claude-plane/internal/server/auth"
	"github.com/claudeplane/claude-plane/internal/server/connmgr"
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

// NewRouter creates a chi router with all API routes configured.
// Public routes (register, login) require no authentication.
// Protected routes (logout, machines) require a valid JWT Bearer token.
func NewRouter(h *Handlers) chi.Router {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", h.Register)
		r.Post("/auth/login", h.Login)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(JWTAuthMiddleware(h.authSvc))
			r.Post("/auth/logout", h.Logout)
			r.Get("/machines", h.ListMachines)
			r.Get("/machines/{machineID}", h.GetMachine)
		})
	})

	return r
}
