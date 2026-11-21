package tailscaled_test

import (
	"os"
	"testing"
)

// shortTempDir is t.TempDir with a path short enough for a Unix socket:
// macOS limits a socket path to 104 bytes.
func shortTempDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "ts") //nolint:usetesting // path length
	if err != nil {
		t.Fatalf("create a temporary directory: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return dir
}
