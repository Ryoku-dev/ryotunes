package diag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLineFormattingAndCaps(t *testing.T) {
	h, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, time.November, 8, 9, 10, 11, 123_000_000, time.FixedZone("offset", 3600))
	h.record("warn", "daemon", "rpc:play", "could not play", at)
	content, err := os.ReadFile(h.state.path)
	if err != nil {
		t.Fatal(err)
	}
	want := "2026-11-08T08:10:11.123Z [WARN] daemon rpc:play: could not play\n"
	if string(content) != want {
		t.Fatalf("line = %q, want %q", content, want)
	}
}

func TestRotationAtThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte(strings.Repeat("x", fileLimit)), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	h.RecordClient("new line", "client", "warn")
	rotated, err := os.Stat(filepath.Join(dir, rotatedName))
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Size() != fileLimit {
		t.Fatalf("rotated size = %d, want %d", rotated.Size(), fileLimit)
	}
	active, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if active.Size() == 0 || active.Size() >= fileLimit {
		t.Fatalf("active size after rotation = %d", active.Size())
	}
}

func TestQueryFiltersNewestFirst(t *testing.T) {
	h, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	h.record("info", "daemon", "startup", "ready", base)
	h.record("warn", "client", "player", "Buffer low", base.Add(time.Second))
	h.record("error", "daemon", "rpc:play", "BUFFER failed", base.Add(2*time.Second))

	got := h.Query("warn", "all", "buffer", 99)
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(got.Entries))
	}
	if got.Entries[0].Level != "error" || got.Entries[1].Level != "warn" {
		t.Fatalf("entries not newest first: %#v", got.Entries)
	}
	if got.Total != 3 || got.Errors != 1 || got.Warnings != 1 {
		t.Fatalf("counts = total %d errors %d warnings %d", got.Total, got.Errors, got.Warnings)
	}

	got = h.Query("error", "daemon", "rpc", -20)
	if len(got.Entries) != 1 || got.Entries[0].Target != "rpc:play" {
		t.Fatalf("filtered entries = %#v", got.Entries)
	}
}
