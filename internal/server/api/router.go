package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"

	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/session"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// Handlers holds the dependencies required by all HTTP handlers.
type Handlers struct {
	store            *store.Store
	authSvc          *auth.Service
	connMgr          *connmgr.ConnectionManager
	registrationMode string
	inviteCode       string
}

// NewHandlers creates a new Handlers instance with the given dependencies.
// registrationMode controls self-registration: "open" (anyone), "invite" (requires code), "closed" (disabled).
// Defaults to "closed" if empty.
func NewHandlers(s *store.Store, authSvc *auth.Service, connMgr *connmgr.ConnectionManager, registrationMode, inviteCode string) *Handlers {
	if registrationMode == "" {
		registrationMode = "closed"
	}
	return &Handlers{
		store:            s,
		authSvc:          authSvc,
		connMgr:          connMgr,
		registrationMode: registrationMode,
		inviteCode:       inviteCode,
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

// RouterDeps holds all dependencies required by NewRouter, replacing the
// previous 15 positional parameters with named fields.
type RouterDeps struct {
	Handlers           *Handlers
	SessionHandler     *session.SessionHandler
	WSHandler          http.HandlerFunc
	EventsWSHandler    http.HandlerFunc
	JobHandler         *handler.JobHandler
	RunHandler         *handler.RunHandler
	EventHandler       *handler.EventHandler
	WebhookHandler     *handler.WebhookHandler
	TriggerHandler     *handler.TriggerHandler
	IngestHandler      *handler.IngestHandler
	ScheduleHandler    *handler.ScheduleHandler
	UserHandler        *handler.UserHandler
	CredentialHandler  *handler.CredentialHandler
	PreferencesHandler *handler.PreferencesHandler
	APIKeyAuth         *APIKeyAuth
}

// NewRouter creates a chi router with all API routes configured.
// Public routes (register, login) require no authentication.
// Protected routes (logout, machines, sessions) require a valid JWT, checked
// via httpOnly cookie first, then Authorization: Bearer header as fallback.
// WebSocket routes support cookie auth (preferred) and first-message auth.
func NewRouter(deps RouterDeps) chi.Router {
	if deps.Handlers == nil {
		panic("api.NewRouter: RouterDeps.Handlers is required")
	}
	h := deps.Handlers
	r := chi.NewRouter()

	aka := deps.APIKeyAuth

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(maxBytesMiddleware(1 << 20)) // 1MB global limit
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(securityHeadersMiddleware)

	// WebSocket routes — auth handled inside handlers (cookie or first-message)
	if deps.WSHandler != nil {
		r.Get("/ws/terminal/{sessionID}", deps.WSHandler)
	}
	if deps.EventsWSHandler != nil {
		r.Get("/ws/events", deps.EventsWSHandler)
	}

	// 5 requests per minute per IP for auth endpoints
	authLimiter := RateLimitMiddleware(rate.Limit(5.0/60.0), 5)

	// Public routes
	r.Route("/api/v1", func(r chi.Router) {
		r.With(authLimiter).Post("/auth/register", h.Register)
		r.With(authLimiter).Post("/auth/login", h.Login)

		// Protected routes (JWT + optional API key auth)
		r.Group(func(r chi.Router) {
			r.Use(JWTAuthMiddleware(h.authSvc, aka))
			r.Get("/auth/me", h.Me)
			r.Post("/auth/logout", h.Logout)
			r.Get("/machines", h.ListMachines)
			r.Get("/machines/{machineID}", h.GetMachine)

			// Session routes
			if deps.SessionHandler != nil {
				r.Post("/sessions", deps.SessionHandler.CreateSession)
				r.Get("/sessions", deps.SessionHandler.ListSessions)
				r.Get("/sessions/{sessionID}", deps.SessionHandler.GetSession)
				r.Delete("/sessions/{sessionID}", deps.SessionHandler.TerminateSession)
				r.Post("/sessions/{sessionID}/inject", deps.SessionHandler.InjectSession)
				r.Get("/sessions/{sessionID}/injections", deps.SessionHandler.ListInjections)
			}
		})
	})

	// Job system routes (flat paths, JWT + optional API key protected)
	if deps.JobHandler != nil || deps.RunHandler != nil || deps.EventHandler != nil || deps.WebhookHandler != nil || deps.TriggerHandler != nil || deps.ScheduleHandler != nil || deps.UserHandler != nil || deps.CredentialHandler != nil || deps.PreferencesHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(JWTAuthMiddleware(h.authSvc, aka))
			if deps.JobHandler != nil {
				handler.RegisterJobRoutes(r, deps.JobHandler)
			}
			if deps.RunHandler != nil {
				handler.RegisterRunRoutes(r, deps.RunHandler)
			}
			if deps.EventHandler != nil {
				handler.RegisterEventRoutes(r, deps.EventHandler)
			}
			if deps.WebhookHandler != nil {
				handler.RegisterWebhookRoutes(r, deps.WebhookHandler)
			}
			if deps.TriggerHandler != nil {
				handler.RegisterTriggerRoutes(r, deps.TriggerHandler)
			}
			if deps.ScheduleHandler != nil {
				handler.RegisterScheduleRoutes(r, deps.ScheduleHandler)
			}
			if deps.UserHandler != nil {
				handler.RegisterUserRoutes(r, deps.UserHandler)
			}
			if deps.CredentialHandler != nil {
				handler.RegisterCredentialRoutes(r, deps.CredentialHandler)
			}
			if deps.PreferencesHandler != nil {
				handler.RegisterPreferencesRoutes(r, deps.PreferencesHandler)
			}
		})
	}

	// Ingest routes are public (auth is handled via HMAC signatures).
	if deps.IngestHandler != nil {
		handler.RegisterIngestRoutes(r, deps.IngestHandler)
	}

	return r
}
