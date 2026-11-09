package spectrum

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStaticScopeKeepsNewestFrames(t *testing.T) {
	samples := make([]float32, (scopeRingFrames+17)*2)
	for frame := range scopeRingFrames + 17 {
		samples[frame*2] = float32(frame)
		samples[frame*2+1] = -float32(frame)
	}
	scope := NewStaticScope(samples)
	defer scope.Close()
	now := time.Now()
	scope.SetActive(true)
	scope.Want(now)

	dst := make([]float32, 8)
	frames, live := scope.Read(dst)
	if !live || frames != 4 {
		t.Fatalf("Read = (%d, %v), want (4, true)", frames, live)
	}
	first := scopeRingFrames + 13
	for frame := range frames {
		want := float32(first + frame)
		if dst[frame*2] != want || dst[frame*2+1] != -want {
			t.Fatalf("frame %d = (%v, %v), want (%v, %v)", frame, dst[frame*2], dst[frame*2+1], want, -want)
		}
	}
}

func TestScopeStartsOnLinkedSinkAndStopsWhenInactive(t *testing.T) {
	binDir := t.TempDir()
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	pidLog := filepath.Join(t.TempDir(), "pw-record-pids")
	argsLog := filepath.Join(t.TempDir(), "pw-record-args")
	rawPath := filepath.Join(t.TempDir(), "samples.raw")
	raw := make([]byte, scopeChunkFrames*scopeChannels*4)
	for frame := range scopeChunkFrames {
		binary.LittleEndian.PutUint32(raw[frame*8:], math.Float32bits(.25))
		binary.LittleEndian.PutUint32(raw[frame*8+4:], math.Float32bits(-.125))
	}
	if err := os.WriteFile(rawPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	pwRecord := "#!/bin/sh\n" +
		"printf '%s\\n' \"$$\" >> \"$RYOTUNES_TEST_SCOPE_PID\"\n" +
		"printf '%s\\n' \"$*\" > \"$RYOTUNES_TEST_SCOPE_ARGS\"\n" +
		"trap 'exit 0' TERM\n" +
		"while :; do /usr/bin/cat \"$RYOTUNES_TEST_SCOPE_RAW\"; /usr/bin/sleep 0.02; done\n"
	writeExecutable(t, filepath.Join(binDir, "pw-record"), pwRecord)
	pwDump := "#!/bin/sh\n/usr/bin/cat <<'EOF'\n" + pipeWireFixture("scope_sink", "ryotunesd", "mpv", "mpv") + "\nEOF\n"
	writeExecutable(t, filepath.Join(binDir, "pw-dump"), pwDump)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("RYOTUNES_TEST_SCOPE_PID", pidLog)
	t.Setenv("RYOTUNES_TEST_SCOPE_ARGS", argsLog)
	t.Setenv("RYOTUNES_TEST_SCOPE_RAW", rawPath)

	scope := NewScope(nil)
	defer scope.Close()
	if !scope.Installed() || !scope.Available() {
		t.Fatal("scope did not find fake pw-record")
	}
	scope.SetActive(true)
	dst := make([]float32, scopeChunkFrames*2)
	waitFor(t, 2*time.Second, func() bool {
		scope.Want(time.Now())
		frames, live := scope.Read(dst)
		return live && frames == scopeChunkFrames && dst[0] == .25 && dst[1] == -.125
	})
	pid := waitForPIDCount(t, pidLog, 1)[0]
	arguments := string(mustRead(t, argsLog))
	for _, required := range []string{"--target scope_sink", "-P { stream.capture.sink = true }", "--rate 48000", "--channels 2", "--format f32", "--raw -"} {
		if !strings.Contains(arguments, required) {
			t.Fatalf("pw-record arguments %q lack %q", arguments, required)
		}
	}

	scope.SetActive(false)
	waitFor(t, time.Second, func() bool {
		return syscall.Kill(pid, 0) == syscall.ESRCH
	})
	if _, live := scope.Read(dst); live {
		t.Fatal("inactive scope remained live")
	}
}

func TestScopeUnavailableWithoutPWRecord(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	scope := NewScope(nil)
	defer scope.Close()
	if scope.Installed() || scope.Available() {
		t.Fatal("scope reports pw-record without pw-record on PATH")
	}
}
func TestScopeResetDropsPreviousProcessFrames(t *testing.T) {
	scope := NewStaticScope([]float32{.25, -.25, .5, -.5})
	defer scope.Close()
	scope.reset()
	dst := make([]float32, 4)
	if frames, _ := scope.Read(dst); frames != 0 {
		t.Fatalf("Read after capture restart returned %d stale frames", frames)
	}
}
