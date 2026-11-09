package shell

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"sync/atomic"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

func (r *clientRuntime) renderPNG(options Options) error {
	window, err := headless.NewWindow(options.Width, options.Height)
	if err != nil {
		return fmt.Errorf("create headless renderer: %w", err)
	}
	defer window.Release()

	preferences := r.store.Snapshot().Client
	initial := initialRoute(preferences.Route, options)
	panel := env.PanelNone
	if look.ClassFor(effectiveInitialWidth(options.Width, preferences)) != look.Compact && preferences.PanelOpen {
		panel = parsePanel(preferences.PanelTab)
	}
	if options.Panel != "" {
		panel = parsePanel(options.Panel)
	}
	shell := newShell(r, initial, panel, options.Mini, options.Width, options.Height)
	if panelOpensNowPlaying(options.Panel) {
		shell.nav.nowPlaying = true
	}

	var dirty atomic.Bool
	dirty.Store(true)
	remove := r.hub.Add(func() { dirty.Store(true) })
	defer remove()
	unsubscribe := r.store.Subscribe(func() { dirty.Store(true) })
	defer unsubscribe()

	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	var operations op.Ops
	var router input.Router
	started := time.Now()
	deadline := started.Add(3 * time.Second)
	quietSince := time.Time{}
	for time.Now().Before(deadline) {
		if !dirty.Swap(false) {
			if quietSince.IsZero() {
				quietSince = time.Now()
			}
			ready := r.store.Snapshot().Provider != ""
			if ready && time.Since(started) >= 300*time.Millisecond && time.Since(quietSince) >= 100*time.Millisecond {
				break
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		quietSince = time.Time{}
		operations.Reset()
		preferences = r.store.Snapshot().Client
		if !options.DemoPlaying {
			preferences.ReduceMotion = true
		}
		scale := float32(preferences.ZoomPercent) / 100
		if scale <= 0 {
			scale = 1
		}
		frameMetric := metric
		frameMetric.PxPerDp *= scale
		frameMetric.PxPerSp *= scale
		gtx := layout.Context{
			Ops: &operations, Now: time.Now(), Source: router.Source(), Metric: frameMetric,
			Constraints: layout.Exact(image.Pt(options.Width, options.Height)),
		}
		theme := r.applyClientTheme(preferences)
		e := shell.environment(gtx.Now, theme)
		act.PendingClipboard(gtx)
		paint.Fill(gtx.Ops, theme.R.Ground)
		if options.Mini {
			shell.reconcileProvider(gtx, e)
			shell.mini.Layout(gtx, e)
			shell.layoutOverlays(gtx, e)
		} else {
			windowWidthDP := float32(gtx.Metric.PxToDp(options.Width))
			shell.layout(gtx, e, windowWidthDP)
			shell.layoutOverlays(gtx, e)
		}
		if err := window.Frame(&operations); err != nil {
			return fmt.Errorf("render PNG frame: %w", err)
		}
		if options.DemoPlaying && time.Since(started) < 600*time.Millisecond {
			time.Sleep(time.Second / 60)
			dirty.Store(true)
		}
	}

	frame := image.NewRGBA(image.Rect(0, 0, options.Width, options.Height))
	if err := window.Screenshot(frame); err != nil {
		return fmt.Errorf("capture PNG frame: %w", err)
	}
	file, err := os.Create(options.PNG)
	if err != nil {
		return fmt.Errorf("create PNG: %w", err)
	}
	if err := png.Encode(file, frame); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode PNG: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close PNG: %w", err)
	}
	return nil
}
