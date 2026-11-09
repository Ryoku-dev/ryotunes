package spectrum

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestStaticFeed(t *testing.T) {
	feed := NewStatic([]float32{-1, 0.25, 2})
	defer feed.Close()
	if !feed.Installed() || !feed.Available() {
		t.Fatal("static feed is not installed and available")
	}

	now := time.Now()
	feed.SetActive(true)
	feed.Want(now)
	levels, live := feed.Levels(now)
	if !live {
		t.Fatal("active wanted static feed is not live")
	}
	if levels[0] != 0 || levels[1] != 0.25 || levels[2] != 1 || len(levels) != Bands {
		t.Fatalf("unexpected static levels: len=%d first=%v", len(levels), levels[:3])
	}
	if _, live := feed.Levels(now.Add(wantLifetime + time.Millisecond)); live {
		t.Fatal("expired static feed remained live")
	}
}

func TestFeedStartsStopsAndRestartsCava(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "cava-pids")
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	frame := string(testFrame())
	cava := "#!/bin/sh\n" +
		"printf '%s\\n' \"$$\" >> \"$RYOTUNES_TEST_CAVA_LOG\"\n" +
		"trap 'exit 0' TERM\n" +
		"while :; do\n" +
		"  printf '%s\\n' \"$RYOTUNES_TEST_FRAME\"\n" +
		"  /usr/bin/sleep 0.02\n" +
		"done\n"
	writeExecutable(t, filepath.Join(binDir, "cava"), cava)
	pwDump := "#!/bin/sh\n/usr/bin/cat <<'EOF'\n" + pipeWireFixture("test_sink", "ryotunesd", "mpv", "mpv") + "\nEOF\n"
	writeExecutable(t, filepath.Join(binDir, "pw-dump"), pwDump)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("RYOTUNES_TEST_CAVA_LOG", logPath)
	t.Setenv("RYOTUNES_TEST_FRAME", frame)

	var invalidations atomic.Int64
	feed := New(func() { invalidations.Add(1) })
	defer feed.Close()
	if !feed.Installed() || !feed.Available() {
		t.Fatal("feed did not find fake cava")
	}
	feed.SetActive(true)

	waitFor(t, 2*time.Second, func() bool {
		now := time.Now()
		feed.Want(now)
		levels, live := feed.Levels(now)
		return live && levels[1] > 0
	})
	first := waitForPIDCount(t, logPath, 1)[0]
	if invalidations.Load() == 0 {
		t.Fatal("feed received levels without invalidating")
	}
	if err := syscall.Kill(first, syscall.SIGKILL); err != nil {
		t.Fatalf("kill first cava: %v", err)
	}

	waitFor(t, 3*time.Second, func() bool {
		feed.Want(time.Now())
		return len(readPIDs(logPath)) >= 2
	})
	second := readPIDs(logPath)[1]
	if second == first {
		t.Fatalf("cava did not restart: pid %d", first)
	}

	feed.SetActive(false)
	if _, live := feed.Levels(time.Now()); live {
		t.Fatal("inactive feed remained live")
	}
	waitFor(t, time.Second, func() bool {
		return syscall.Kill(second, 0) == syscall.ESRCH
	})

	config, err := os.ReadFile(filepath.Join(runtimeDir, "ryotunes", "cava.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "source = test_sink.monitor\n") {
		t.Fatalf("feed did not resolve linked sink:\n%s", config)
	}
}

func TestFeedGivesUpAfterThreeFailedStarts(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "failed-starts")
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cava := "#!/bin/sh\nprintf 'failed\\n' >> \"$RYOTUNES_TEST_CAVA_LOG\"\nexit 1\n"
	writeExecutable(t, filepath.Join(binDir, "cava"), cava)
	writeExecutable(t, filepath.Join(binDir, "pw-dump"), "#!/bin/sh\nprintf '[]\\n'\n")
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("RYOTUNES_TEST_CAVA_LOG", logPath)

	feed := New(nil)
	defer feed.Close()
	feed.SetActive(true)
	waitFor(t, 4*time.Second, func() bool {
		feed.Want(time.Now())
		return !feed.Available()
	})
	if !feed.Installed() {
		t.Fatal("failed capture hid installed cava")
	}
	if starts := len(strings.Fields(string(mustRead(t, logPath)))); starts != failureLimit {
		t.Fatalf("failed starts = %d, want %d", starts, failureLimit)
	}

	feed.SetActive(false)
	feed.SetActive(true)
	waitFor(t, time.Second, feed.Available)
	feed.SetActive(false)
}

func TestFeedSetActiveSignalsOnlyChanges(t *testing.T) {
	feed := NewStatic(nil)
	defer feed.Close()
	for _, active := range []bool{true, false} {
		feed.SetActive(active)
		select {
		case <-feed.capture.wake:
		default:
			t.Fatalf("SetActive(%v) did not signal its state change", active)
		}
		feed.SetActive(active)
		select {
		case <-feed.capture.wake:
			t.Fatalf("SetActive(%v) signalled an unchanged state", active)
		default:
		}
	}
}

func TestFeedInvalidatesOnlyWhenUIIsNotPulling(t *testing.T) {
	var invalidations atomic.Int64
	feed := &Feed{}
	feed.capture = &captureLifecycle{
		invalidate: func() { invalidations.Add(1) },
		active:     true,
		running:    true,
		needsWake:  true,
		lastWant:   time.Now(),
	}
	frame := string(testFrame()) + "\n"
	feed.readFrames(strings.NewReader(frame + frame))
	if got := invalidations.Load(); got != 1 {
		t.Fatalf("invalidations with recent Want = %d, want first-frame wake only", got)
	}

	feed.capture.mu.Lock()
	feed.capture.lastWant = time.Now().Add(-2 * pullInvalidateWindow)
	feed.capture.mu.Unlock()
	feed.readFrames(strings.NewReader(frame))
	if got := invalidations.Load(); got != 2 {
		t.Fatalf("invalidations after pulling stopped = %d, want 2", got)
	}
}

func TestFeedUnavailableWithoutCava(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	feed := New(nil)
	defer feed.Close()
	if feed.Installed() || feed.Available() {
		t.Fatal("feed reports cava without cava on PATH")
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}

func waitForPIDCount(t *testing.T, path string, count int) []int {
	t.Helper()
	var pids []int
	waitFor(t, 2*time.Second, func() bool {
		pids = readPIDs(path)
		return len(pids) >= count
	})
	return pids
}

func readPIDs(path string) []int {
	data, _ := os.ReadFile(path)
	fields := strings.Fields(string(data))
	pids := make([]int, 0, len(fields))
	for _, field := range fields {
		pid, err := strconv.Atoi(field)
		if err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition did not become true before timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
