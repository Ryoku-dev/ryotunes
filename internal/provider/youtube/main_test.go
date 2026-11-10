package youtube

import (
	"os"
	"testing"
)

// TestMain keeps every browser store copy out of the live session's runtime
// directory.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "youtube-runtime-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_RUNTIME_DIR", dir); err != nil {
		panic(err)
	}
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		panic(err)
	}
	os.Exit(code)
}
