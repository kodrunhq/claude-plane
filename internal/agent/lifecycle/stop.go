package lifecycle

import (
	"log/slog"
	"time"
)

// StopExisting performs a three-layer stop of any existing agent instance:
//
//  1. Stop the system service (systemd/launchd) if active.
//  2. Read the PID file — signal a live process or remove a stale file.
//  3. Scan for remaining agent processes and terminate them.
//
// All three layers are attempted regardless of earlier outcomes. Errors are
// logged as warnings so callers can proceed unconditionally.
func StopExisting(dataDir string, logger *slog.Logger) {
	// Layer 1: service manager.
	if stopped := StopServiceIfActive(logger); stopped {
		logger.Info("stopped existing agent via service manager")
	}

	// Layer 2: PID file.
	pid, alive, err := CheckPIDFile(dataDir)
	if err != nil {
		logger.Warn("failed to check pid file", "error", err)
	} else if alive {
		logger.Info("found live agent from pid file", "pid", pid)
		SignalAndWait(pid, 5*time.Second, logger)
		RemovePIDFile(dataDir)
	} else if pid != 0 {
		logger.Info("removing stale pid file", "pid", pid)
		RemovePIDFile(dataDir)
	}

	// Layer 3: process scan.
	pids, err := FindAgentProcesses()
	if err != nil {
		logger.Warn("failed to scan for agent processes", "error", err)
	} else {
		for _, p := range pids {
			logger.Info("stopping discovered agent process", "pid", p)
			SignalAndWait(p, 3*time.Second, logger)
		}
	}
}
