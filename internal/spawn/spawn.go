// Package spawn starts programs that are not part of the calling process.
package spawn

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Command returns name with args in a new session. Under a systemd user
// manager it runs inside its own transient scope in app.slice, so a browser
// or window started from ryotunesd outlives the daemon's unit instead of being
// killed with it when the daemon idles out or restarts.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	var command *exec.Cmd
	if runner, ok := scopeRunner(); ok {
		command = exec.CommandContext(ctx, runner, append([]string{"--user", "--scope", "--collect", "--quiet", "--slice=app.slice", "--", name}, args...)...)
	} else {
		command = exec.CommandContext(ctx, name, args...)
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return command
}

func scopeRunner() (string, bool) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return "", false
	}
	info, err := os.Stat(filepath.Join(runtimeDir, "systemd", "private"))
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return "", false
	}
	runner, err := exec.LookPath("systemd-run")
	return runner, err == nil
}
