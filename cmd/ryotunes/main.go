package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	gioapp "gioui.org/app"

	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/shell"
)

func main() {
	code, err := execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ryotunes:", err)
	}
	if code != 0 {
		os.Exit(code)
	}
}

func execute() (int, error) {
	pngPath := flag.String("png", "", "render one frame to a PNG and exit")
	size := flag.String("size", "", "window or PNG size as WIDTHxHEIGHT (default 1280x800)")
	demoMode := flag.Bool("demo", false, "use the in-process demo daemon")
	demoPlaying := flag.Bool("demo-playing", false, "play the demo fixture for live visual effects")
	route := flag.String("route", "", "initial route (default home)")
	query := flag.String("query", "", "initial search query")
	panel := flag.String("panel", "", "initial panel: queue, lyrics, now-playing, listen-together, or empty")
	mode := flag.String("mode", "", "palette mode override: follow, dark, or light")
	mini := flag.Bool("mini", false, "open the mini player")
	flag.Parse()

	width, height := 0, 0
	if *size != "" {
		var err error
		width, height, err = parseSize(*size)
		if err != nil {
			return 2, err
		}
	}

	var clientLock *state.ClientLock
	if !*demoMode && *pngPath == "" {
		var err error
		clientLock, err = state.AcquireClientLock()
		if errors.Is(err, state.ErrClientLockHeld) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = conn.Show(ctx)
			return 0, nil
		}
		if err != nil {
			return 1, err
		}
	}

	options := shell.Options{
		Demo: *demoMode, DemoPlaying: *demoPlaying, PNG: *pngPath, Width: width, Height: height,
		Route: *route, Query: *query, Panel: *panel, Mode: *mode, Mini: *mini,
	}
	if *pngPath != "" {
		if err := shell.Run(options); err != nil {
			return 1, err
		}
		return 0, nil
	}

	go exitAfterShell(options, clientLock)
	gioapp.Main()
	return 0, nil
}

func exitAfterShell(options shell.Options, clientLock *state.ClientLock) {
	err := shell.Run(options)
	if closeErr := clientLock.Close(); err == nil && closeErr != nil {
		err = fmt.Errorf("release client lock: %w", closeErr)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ryotunes:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func parseSize(value string) (int, int, error) {
	widthText, heightText, ok := strings.Cut(strings.ToLower(strings.TrimSpace(value)), "x")
	if !ok {
		return 0, 0, fmt.Errorf("invalid --size %q: use WIDTHxHEIGHT", value)
	}
	width, widthErr := strconv.Atoi(widthText)
	height, heightErr := strconv.Atoi(heightText)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("invalid --size %q: use positive dimensions", value)
	}
	return width, height, nil
}
