package agent

import "testing"

func TestMergePaths(t *testing.T) {
	tests := []struct {
		name    string
		login   string
		current string
		want    string
	}{
		{
			name:    "login adds new dirs",
			login:   "/home/user/.nvm/bin:/usr/bin",
			current: "/usr/bin:/usr/local/bin",
			want:    "/home/user/.nvm/bin:/usr/bin:/usr/local/bin",
		},
		{
			name:    "deduplicates",
			login:   "/usr/bin:/usr/bin",
			current: "/usr/bin",
			want:    "/usr/bin",
		},
		{
			name:    "empty login path",
			login:   "",
			current: "/usr/bin",
			want:    "/usr/bin",
		},
		{
			name:    "empty current path",
			login:   "/home/user/.local/bin",
			current: "",
			want:    "/home/user/.local/bin",
		},
		{
			name:    "login paths take priority",
			login:   "/home/user/.local/bin:/usr/bin",
			current: "/usr/local/bin:/usr/bin",
			want:    "/home/user/.local/bin:/usr/bin:/usr/local/bin",
		},
		{
			name:    "both empty",
			login:   "",
			current: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergePaths(tt.login, tt.current)
			if got != tt.want {
				t.Errorf("mergePaths(%q, %q) = %q, want %q", tt.login, tt.current, got, tt.want)
			}
		})
	}
}

func TestCountNewDirs(t *testing.T) {
	before := "/usr/bin:/usr/local/bin"
	after := "/home/user/.nvm/bin:/usr/bin:/usr/local/bin"
	got := countNewDirs(before, after)
	if got != 1 {
		t.Errorf("countNewDirs = %d, want 1", got)
	}
}
