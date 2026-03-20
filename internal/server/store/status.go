package store

import "github.com/kodrunhq/claude-plane/internal/shared/status"

// Status string constants re-exported from the shared status package.
// Server code can use store.StatusRunning etc. as before.
const (
	StatusPending         = status.Pending
	StatusCreated         = status.Created
	StatusStarting        = status.Starting
	StatusRunning         = status.Running
	StatusCompleted       = status.Completed
	StatusFailed          = status.Failed
	StatusSkipped         = status.Skipped
	StatusCancelled       = status.Cancelled
	StatusTerminated      = status.Terminated
	StatusWaitingForInput = status.WaitingForInput
)
