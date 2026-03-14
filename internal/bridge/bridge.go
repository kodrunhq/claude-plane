// Package bridge wires together all bridge connectors and manages their lifecycle.
// It starts an HTTP health endpoint, polls for restart signals from the server,
// and gracefully shuts down connectors when the context is cancelled or a restart
// is requested.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge/connector"
	"github.com/kodrunhq/claude-plane/internal/bridge/state"
)

const defaultPollInterval = 15 * time.Second

// APIClient is the subset of *client.Client that the Bridge requires.
// Declaring a narrow interface here keeps the bridge decoupled from the
// concrete HTTP client and makes unit testing straightforward.
type APIClient interface {
	CheckRestartSignal(ctx context.Context, bootTime time.Time) (bool, error)
}

// Bridge orchestrates one or more Connectors and the bridge process lifecycle.
type Bridge struct {
	client     APIClient
	state      *state.Store
	connectors []connector.Connector
	healthAddr string
	logger     *slog.Logger
	bootTime   time.Time
}

// New creates a Bridge. Pass a nil logger to use slog.Default().
func New(client APIClient, st *state.Store, healthAddr string, logger *slog.Logger) *Bridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bridge{
		client:     client,
		state:      st,
		connectors: make([]connector.Connector, 0),
		healthAddr: healthAddr,
		logger:     logger,
	}
}

// AddConnector registers a connector to be started by Run.
// It must be called before Run.
func (b *Bridge) AddConnector(c connector.Connector) {
	b.connectors = append(b.connectors, c)
}

// Run starts the bridge: it records the boot time, starts the health endpoint,
// launches all connectors, and polls for a restart signal every 15 seconds.
// It returns nil when the context is cancelled or when a restart signal is received.
// Use RunWithInterval for testing with a shorter poll interval.
func (b *Bridge) Run(ctx context.Context) error {
	return b.RunWithInterval(ctx, defaultPollInterval)
}

// RunWithInterval is like Run but uses the provided interval for restart-signal polling.
// It is exported for testing purposes.
func (b *Bridge) RunWithInterval(ctx context.Context, pollInterval time.Duration) error {
	b.bootTime = time.Now()

	// Apply default health address if not configured.
	if b.healthAddr == "" {
		b.healthAddr = "localhost:9091"
		b.logger.Warn("health address not configured, defaulting to localhost:9091")
	}

	// Start the health HTTP server.
	healthSrv, healthErrCh := b.startHealthServer()

	// Give the health server 100 ms to confirm it bound successfully.
	select {
	case err := <-healthErrCh:
		return err
	case <-time.After(100 * time.Millisecond):
		b.logger.Info("health server started", "addr", b.healthAddr)
	}

	// Derive a child context so we can cancel connectors independently.
	connCtx, cancelConn := context.WithCancel(ctx)
	defer cancelConn()

	// Start each connector in its own goroutine.
	var wg sync.WaitGroup
	for _, c := range b.connectors {
		c := c // capture loop variable
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Start(connCtx); err != nil {
				b.logger.Error("connector exited with error",
					"connector", c.Name(),
					"error", err,
				)
			}
		}()
	}

	// Poll for restart signal until context is done.
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var runErr error
loop:
	for {
		select {
		case <-ctx.Done():
			b.logger.Info("context cancelled, shutting down bridge")
			break loop
		case <-ticker.C:
			restart, err := b.client.CheckRestartSignal(ctx, b.bootTime)
			if err != nil {
				b.logger.Warn("failed to check restart signal", "error", err)
				continue
			}
			if restart {
				b.logger.Info("restart signal received, shutting down")
				break loop
			}
		}
	}

	// Cancel all connectors and wait up to 10 seconds for them to finish.
	cancelConn()

	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(10 * time.Second):
		b.logger.Warn("timed out waiting for connectors to stop")
	}

	// Shut down the health server.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		b.logger.Warn("health server shutdown error", "error", err)
	}

	return runErr
}

// startHealthServer creates and starts the HTTP health endpoint in a goroutine.
// It returns the *http.Server so the caller can shut it down cleanly, and a
// buffered error channel that receives any startup failure.
func (b *Bridge) startHealthServer() (*http.Server, <-chan error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", b.handleHealthz)

	srv := &http.Server{
		Addr:    b.healthAddr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("health server failed to start on %s: %w", b.healthAddr, err)
		}
	}()

	return srv, errCh
}

// handleHealthz writes a JSON health response including per-connector status.
func (b *Bridge) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	connectorStatuses := make([]map[string]interface{}, 0, len(b.connectors))
	for _, c := range b.connectors {
		connectorStatuses = append(connectorStatuses, map[string]interface{}{
			"name":    c.Name(),
			"healthy": c.Healthy(),
		})
	}

	status := map[string]interface{}{
		"status":     "ok",
		"boot_time":  b.bootTime.Format(time.RFC3339),
		"connectors": connectorStatuses,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		b.logger.Warn("failed to encode health response", "error", err)
	}
}
