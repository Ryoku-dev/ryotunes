package spawn

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCommandRunsInItsOwnScopeOnlyUnderAUserManager(t *testing.T) {
	bin := t.TempDir()
	runner := filepath.Join(bin, "systemd-run")
	if err := os.WriteFile(runner, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	direct := []string{"xdg-open", "https://example.org"}
	if got := Command(context.Background(), "xdg-open", "https://example.org").Args; !slices.Equal(got, direct) {
		t.Fatalf("without a user manager Args = %q, want %q", got, direct)
	}

	if err := os.Mkdir(filepath.Join(runtimeDir, "systemd"), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(runtimeDir, "systemd", "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	scoped := []string{runner, "--user", "--scope", "--collect", "--quiet", "--slice=app.slice", "--", "xdg-open", "https://example.org"}
	if got := Command(context.Background(), "xdg-open", "https://example.org").Args; !slices.Equal(got, scoped) {
		t.Fatalf("under a user manager Args = %q, want %q", got, scoped)
	}

	t.Setenv("PATH", t.TempDir())
	if got := Command(context.Background(), "xdg-open", "https://example.org").Args; !slices.Equal(got, direct) {
		t.Fatalf("without systemd-run Args = %q, want %q", got, direct)
	}
}
