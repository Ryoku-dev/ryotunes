package spectrum

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConfig(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	path, err := writeConfig("easyeffects_sink")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(runtimeDir, "ryotunes", "cava.conf"); path != want {
		t.Fatalf("config path = %q, want %q", path, want)
	}
	assertMode(t, filepath.Dir(path), configDirectoryMode)
	assertMode(t, path, configFileMode)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"[general]", "framerate = 60", "bars = 64", "autosens = 1",
		"[input]", "method = pipewire", "source = easyeffects_sink.monitor",
		"[output]", "method = raw", "raw_target = /dev/stdout", "data_format = ascii",
		"ascii_max_range = 1000", "bar_delimiter = 59", "frame_delimiter = 10",
		"channels = mono", "mono_option = average", "[smoothing]", "noise_reduction = 30",
	} {
		if !strings.Contains(string(data), line+"\n") {
			t.Errorf("config is missing %q", line)
		}
	}
}

func TestWriteConfigRequiresRuntimeDirectory(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	if _, err := writeConfig(automaticSource); err == nil {
		t.Fatal("writeConfig succeeded without XDG_RUNTIME_DIR")
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}
