package lifecycle

import (
	"log/slog"
	"os"
	"testing"
)

func TestFindAgentProcesses_ExcludesSelf(t *testing.T) {
	currentPID := os.Getpid()

	pids, err := FindAgentProcesses()
	if err != nil {
		t.Fatalf("FindAgentProcesses() returned error: %v", err)
	}

	for _, pid := range pids {
		if pid == currentPID {
			t.Errorf("FindAgentProcesses() returned current PID %d; should be excluded", currentPID)
		}
	}
}

func TestHasPPID1_True(t *testing.T) {
	status := "Name:\tsome_proc\nPid:\t1234\nPPid:\t1\nUid:\t1000\n"
	if !hasPPID1(status) {
		t.Error("hasPPID1() returned false for PPid:1; want true")
	}
}

func TestHasPPID1_False(t *testing.T) {
	status := "Name:\tsome_proc\nPid:\t1234\nPPid:\t5678\nUid:\t1000\n"
	if hasPPID1(status) {
		t.Error("hasPPID1() returned true for PPid:5678; want false")
	}
}

func TestHasPPID1_Missing(t *testing.T) {
	status := "Name:\tsome_proc\nPid:\t1234\nUid:\t1000\n"
	if hasPPID1(status) {
		t.Error("hasPPID1() returned true when PPid line is missing; want false")
	}
}

func TestParseProcCmdline(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want []string
	}{
		{
			name: "normal args",
			data: []byte("/usr/bin/claude-plane-agent\x00run\x00--config\x00agent.toml\x00"),
			want: []string{"/usr/bin/claude-plane-agent", "run", "--config", "agent.toml"},
		},
		{
			name: "empty",
			data: []byte{},
			want: nil,
		},
		{
			name: "single arg no trailing null",
			data: []byte("claude"),
			want: []string{"claude"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProcCmdline(tt.data)
			if len(got) != len(tt.want) {
				t.Fatalf("parseProcCmdline() = %v; want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseProcCmdline()[%d] = %q; want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMatchesAgentRunCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"full path with run", []string{"/usr/bin/claude-plane-agent", "run"}, true},
		{"with flags", []string{"claude-plane-agent", "run", "--config", "a.toml"}, true},
		{"no run subcommand", []string{"claude-plane-agent", "version"}, false},
		{"wrong binary", []string{"other-agent", "run"}, false},
		{"empty", nil, false},
		{"single arg", []string{"claude-plane-agent"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesAgentRunCommand(tt.args); got != tt.want {
				t.Errorf("matchesAgentRunCommand(%v) = %v; want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestMatchesClaudeCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"claude binary", []string{"/usr/bin/claude", "--help"}, true},
		{"claude binary bare", []string{"claude"}, true},
		{"claude-plane-agent excluded", []string{"claude-plane-agent", "run"}, false},
		{"claude-plane-server excluded", []string{"claude-plane-server", "serve"}, false},
		{"unrelated", []string{"/usr/bin/bash"}, false},
		{"empty", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesClaudeCommand(tt.args); got != tt.want {
				t.Errorf("matchesClaudeCommand(%v) = %v; want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParsePIDList(t *testing.T) {
	data := []byte("100\n200\n300\n")
	pids, err := parsePIDList(data, 200)
	if err != nil {
		t.Fatalf("parsePIDList() error: %v", err)
	}
	if len(pids) != 2 {
		t.Fatalf("parsePIDList() returned %d pids; want 2", len(pids))
	}
	if pids[0] != 100 || pids[1] != 300 {
		t.Errorf("parsePIDList() = %v; want [100, 300]", pids)
	}
}

func TestReapOrphanedProcesses_NoPanic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	// Should not panic.
	ReapOrphanedProcesses(logger)
}
