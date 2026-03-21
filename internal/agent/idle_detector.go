package agent

import (
	"sync"
	"time"
)

const (
	DefaultSilenceTimeout   = 10 * time.Second
	DefaultMinActivityBytes = 10
	DefaultStartupTimeout   = 60 * time.Second
)

type IdleDetector struct {
	silenceTimeout   time.Duration
	minActivityBytes int
	startupTimeout   time.Duration

	onIdle   func()
	onActive func()

	mu           sync.Mutex
	timer        *time.Timer
	startupTimer *time.Timer
	isIdle       bool
	outputSeen   bool
	stopped      bool
}

type IdleDetectorOption func(*IdleDetector)

func WithSilenceTimeout(d time.Duration) IdleDetectorOption {
	return func(det *IdleDetector) {
		if d > 0 {
			det.silenceTimeout = d
		}
	}
}

func WithMinActivityBytes(n int) IdleDetectorOption {
	return func(det *IdleDetector) {
		if n > 0 {
			det.minActivityBytes = n
		}
	}
}

func WithStartupTimeout(d time.Duration) IdleDetectorOption {
	return func(det *IdleDetector) {
		if d > 0 {
			det.startupTimeout = d
		}
	}
}

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

func (d *IdleDetector) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopped = false
	d.isIdle = false
	d.outputSeen = false

	d.startupTimer = time.AfterFunc(d.startupTimeout, func() {
		d.mu.Lock()
		if d.stopped || d.outputSeen {
			d.mu.Unlock()
			return
		}
		d.isIdle = true
		d.mu.Unlock()
		d.onIdle()
	})
}

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

func (d *IdleDetector) Feed(data []byte) {
	if len(data) < d.minActivityBytes {
		return
	}

	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}

	if !d.outputSeen {
		d.outputSeen = true
		if d.startupTimer != nil {
			d.startupTimer.Stop()
		}
	}

	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.silenceTimeout, func() {
		d.mu.Lock()
		if d.stopped {
			d.mu.Unlock()
			return
		}
		d.isIdle = true
		d.mu.Unlock()
		d.onIdle()
	})

	wasIdle := d.isIdle
	if wasIdle {
		d.isIdle = false
	}
	d.mu.Unlock()

	if wasIdle && d.onActive != nil {
		d.onActive()
	}
}
