package buildinfo

// These variables are set at build time via -ldflags.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return "claude-plane " + Version + " (" + Commit + ", " + Date + ")"
}
