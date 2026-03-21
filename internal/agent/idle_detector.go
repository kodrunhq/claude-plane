package agent

import (
	"sync"
	"time"
)

const (
	// DefaultSilenceTimeout is how long the detector waits with no meaningful
	// output before considering the session idle.
	DefaultSilenceTimeout = 10 * time.Second

	// DefaultMinActivityBytes is the minimum data size that counts as
	// meaningful output. Smaller chunks (e.g., cursor escape sequences)
	// are ignored.
	DefaultMinActivityBytes = 10

	// DefaultStartupTimeout is how long to wait for the CLI to produce
	// any output before assuming it is idle.
	DefaultStartupTimeout = 60 * time.Second
)

// IdleDetector watches PTY output volume to determine when a CLI session
// is idle. It fires onIdle when no meaningful output (>= minActivityBytes)
// arrives within silenceTimeout, and fires onActive when output resumes
// after an idle period.
type IdleDetector struct {
	silenceTimeout   time.Duration
	minActivityBytes int
	startupTimeout   time.Duration

	onIdle   func()
	onActive func() // may be nil

	mu           sync.Mutex
	timer        *time.Timer // silence timer — reset on each meaningful Feed
	startupTimer *time.Timer // fires if no output at all within startupTimeout
	isIdle       bool
	outputSeen   bool
	stopped      bool
}

// IdleDetectorOption configures optional IdleDetector settings.
type IdleDetectorOption func(*IdleDetector)

// WithSilenceTimeout overrides the default silence timeout.
func WithSilenceTimeout(d time.Duration) IdleDetectorOption {
	return func(det *IdleDetector) {
		if d > 0 {
			det.silenceTimeout = d
		}
	}
}

// WithMinActivityBytes overrides the minimum data size that counts as activity.
func WithMinActivityBytes(n int) IdleDetectorOption {
	return func(det *IdleDetector) {
		if n > 0 {
			det.minActivityBytes = n
		}
	}
}

// WithStartupTimeout overrides the default startup timeout.
func WithStartupTimeout(d time.Duration) IdleDetectorOption {
	return func(det *IdleDetector) {
		if d > 0 {
			det.startupTimeout = d
		}
	}
}

// NewIdleDetector creates a detector that watches for silence in PTY output.
// onIdle fires when silence exceeds the threshold (active → idle).
// onActive fires when output resumes after an idle period (idle → active).
// onActive may be nil.
func NewIdleDetector(onIdle func(), onActive func(), opts ...IdleDetectorOption) *IdleDetector {
	d := &IdleDetector{
		silenceTimeout:   DefaultSilenceTimeout,
		minActivityBytes: DefaultMinActivityBytes,
		startupTimeout:   DefaultStartupTimeout,
		onIdle:           onIdle,
		onActive:         onActive,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Start begins the startup timeout timer. Call Stop() to clean up.
func (d *IdleDetector) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopped = false
	d.isIdle = false
	d.outputSeen = false

	d.startupTimer = time.AfterFunc(d.startupTimeout, func() {
		d.mu.Lock()
		skip := d.stopped || d.outputSeen
		if !skip {
			d.isIdle = true
		}
		d.mu.Unlock()
		if !skip {
			d.onIdle()
		}
	})
}

// Stop cancels all timers. Safe to call multiple times.
func (d *IdleDetector) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
	}
	if d.startupTimer != nil {
		d.startupTimer.Stop()
	}
}

// Feed processes a chunk of PTY output. Chunks smaller than minActivityBytes
// are ignored (filters cursor noise). Meaningful output resets the silence
// timer and transitions the detector from idle to active if applicable.
func (d *IdleDetector) Feed(data []byte) {
	d.mu.Lock()
	if d.stopped || len(data) < d.minActivityBytes {
		d.mu.Unlock()
		return
	}

	// Cancel startup timer on first meaningful output.
	if !d.outputSeen {
		d.outputSeen = true
		if d.startupTimer != nil {
			d.startupTimer.Stop()
		}
	}

	// Reset (or create) the silence timer.
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.silenceTimeout, func() {
		d.mu.Lock()
		skip := d.stopped
		if !skip {
			d.isIdle = true
		}
		d.mu.Unlock()
		if !skip {
			d.onIdle()
		}
	})

	// Transition: idle → active.
	wasIdle := d.isIdle
	if wasIdle {
		d.isIdle = false
	}
	d.mu.Unlock()

	if wasIdle && d.onActive != nil {
		d.onActive()
	}
}
