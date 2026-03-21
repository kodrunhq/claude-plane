package agent

import (
	"bytes"
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

// Cursor positioning escape sequences used by Ink (React for terminals)
// to re-render status areas below the prompt. These indicate UI chrome
// redraws, not real Claude response output.
var (
	// DECSC — save cursor position.
	cursorSave = []byte{0x1b, '7'}
	// DECRC — restore cursor position.
	cursorRestore = []byte{0x1b, '8'}
	// CSI prefix for cursor movement commands (CUP, CUU, CUD, etc.).
	csiPrefix = []byte{0x1b, '['}
)

// isRepositioningNoise returns true if the data chunk contains cursor
// positioning escape sequences that indicate an Ink status bar redraw
// rather than real Claude response output.
//
// Ink (Claude CLI's terminal renderer) updates status areas below the
// prompt by saving the cursor, jumping to an absolute position, writing
// the update, and restoring the cursor. This produces sequences like:
//
//	\x1b7              (save cursor)
//	\x1b[14;1H         (move to row 14, col 1)
//	\x1b[2K            (erase line)
//	...status text...
//	\x1b8              (restore cursor)
//
// Real Claude response output only uses SGR color codes (\x1b[...m) and
// sequential forward-flowing text — it never repositions the cursor.
func isRepositioningNoise(data []byte) bool {
	// Fast path: save/restore cursor is the strongest Ink signal.
	if bytes.Contains(data, cursorSave) || bytes.Contains(data, cursorRestore) {
		return true
	}

	// Check for CSI cursor movement: \x1b[<digits>A (up), \x1b[<digits>;<digits>H (absolute).
	// SGR color codes end in 'm', so we look for CSI sequences that end in
	// a cursor movement command letter instead.
	idx := 0
	for {
		pos := bytes.Index(data[idx:], csiPrefix)
		if pos < 0 {
			break
		}
		seqStart := idx + pos + len(csiPrefix)
		// Walk past digits and semicolons to find the command byte.
		i := seqStart
		for i < len(data) && ((data[i] >= '0' && data[i] <= '9') || data[i] == ';') {
			i++
		}
		if i < len(data) {
			cmd := data[i]
			// Cursor movement commands: A=up, B=down, C=forward, D=back,
			// H/f=absolute position, J=erase display, K=erase line.
			// SGR is 'm' — we explicitly exclude it.
			switch cmd {
			case 'A', 'B', 'H', 'f', 'J', 'K':
				return true
			}
		}
		idx = seqStart
	}

	return false
}

// IdleDetector watches PTY output to determine when a CLI session is idle.
// It uses a silence timer that resets on meaningful output. Output containing
// cursor positioning escape sequences (Ink status bar redraws) is classified
// as noise and ignored, preventing false "active" transitions from CLI chrome
// updates while the session is actually idle at the prompt.
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
// are ignored. Chunks containing cursor positioning escape sequences are
// classified as Ink status bar noise and ignored. Only sequential text output
// (real Claude responses) resets the silence timer.
func (d *IdleDetector) Feed(data []byte) {
	d.mu.Lock()
	if d.stopped || len(data) < d.minActivityBytes || isRepositioningNoise(data) {
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
