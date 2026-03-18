package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// maxDirectoryEntries is the cap on directory entries returned.
const maxDirectoryEntries = 500

// handleListDirectory reads a directory and sends a DirectoryListingEvent response.
// It validates the path is absolute, reads the directory contents, and caps the
// result at maxDirectoryEntries.
func (c *AgentClient) handleListDirectory(cmd *pb.ListDirectoryCmd, sendCh chan<- *pb.AgentEvent) {
	requestID := cmd.GetRequestId()
	dirPath := filepath.Clean(cmd.GetPath())

	// Reject non-absolute paths.
	if !filepath.IsAbs(dirPath) {
		c.sendDirectoryError(sendCh, requestID, dirPath, fmt.Sprintf("path must be absolute: %s", dirPath))
		return
	}

	// Restrict browsing to the user's home directory.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		c.logger.Debug("could not determine home directory", "error", err)
	}
	if homeDir != "" {
		if !strings.HasPrefix(dirPath, homeDir+"/") && dirPath != homeDir {
			c.sendDirectoryError(sendCh, requestID, dirPath, "path outside allowed directory")
			return
		}
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		c.sendDirectoryError(sendCh, requestID, dirPath, fmt.Sprintf("read directory: %v", err))
		return
	}

	// Cap entries.
	if len(entries) > maxDirectoryEntries {
		entries = entries[:maxDirectoryEntries]
	}

	pbEntries := make([]*pb.DirectoryEntry, 0, len(entries))
	for _, e := range entries {
		entryType := "file"
		if e.IsDir() {
			entryType = "dir"
		}
		pbEntries = append(pbEntries, &pb.DirectoryEntry{
			Name: e.Name(),
			Type: entryType,
		})
	}

	// Compute parent directory.
	parent := filepath.Dir(dirPath)

	evt := &pb.AgentEvent{
		Event: &pb.AgentEvent_DirectoryListing{
			DirectoryListing: &pb.DirectoryListingEvent{
				RequestId: requestID,
				Path:      dirPath,
				Entries:   pbEntries,
				Parent:    parent,
			},
		},
	}

	select {
	case sendCh <- evt:
	default:
		c.logger.Warn("send channel full, dropping directory listing event", "request_id", requestID)
	}
}

// sendDirectoryError sends a DirectoryListingEvent with an error message.
func (c *AgentClient) sendDirectoryError(sendCh chan<- *pb.AgentEvent, requestID, path, errMsg string) {
	evt := &pb.AgentEvent{
		Event: &pb.AgentEvent_DirectoryListing{
			DirectoryListing: &pb.DirectoryListingEvent{
				RequestId: requestID,
				Path:      path,
				Error:     errMsg,
			},
		},
	}

	select {
	case sendCh <- evt:
	default:
		c.logger.Warn("send channel full, dropping directory error event", "request_id", requestID)
	}
}
