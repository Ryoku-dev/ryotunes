package look

import (
	"image"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	clientart "github.com/ryoku-dev/ryotunes/internal/client/art"
)

func TestRevealMaskGeometry(t *testing.T) {
	size := image.Pt(100, 100)
	if _, count := revealMask(size, 0); count != 0 {
		t.Fatalf("progress 0 count = %d, want 0", count)
	}
	half, count := revealMask(size, .5)
	wantHalf := [...]f32.Point{f32.Pt(0, 0), f32.Pt(100, 0), f32.Pt(0, 100)}
	if count != len(wantHalf) {
		t.Fatalf("progress .5 count = %d, want %d", count, len(wantHalf))
	}
	for index, want := range wantHalf {
		if half[index] != want {
			t.Fatalf("progress .5 point %d = %v, want %v", index, half[index], want)
		}
	}
	full, count := revealMask(size, 1)
	wantFull := [...]f32.Point{f32.Pt(0, 0), f32.Pt(100, 0), f32.Pt(100, 100), f32.Pt(0, 100)}
	if count != len(wantFull) {
		t.Fatalf("progress 1 count = %d, want %d", count, len(wantFull))
	}
	for index, want := range wantFull {
		if full[index] != want {
			t.Fatalf("progress 1 point %d = %v, want %v", index, full[index], want)
		}
	}
}

func TestArtViewDropsOutgoingCoverWhenArtworkCannotLoad(t *testing.T) {
	var operations op.Ops
	gtx := layout.Context{
		Ops:         &operations,
		Now:         time.Unix(1000, 0),
		Constraints: layout.Exact(image.Pt(64, 64)),
	}
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	view := ArtView{current: "loaded-cover", currentLoaded: true, ready: true}
	view.Layout(gtx, theme, nil, "", image.Pt(64, 64), Shape{}, theme.Motion.Field)
	if view.previous != "" {
		t.Fatalf("empty artwork retained outgoing cover %q", view.previous)
	}

	changed := make(chan struct{}, 1)
	cache := clientart.New(clientart.Options{Workers: 1, Invalidate: func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}})
	defer cache.Close()
	missing := filepath.Join(t.TempDir(), "missing.png")
	cache.Get(missing, 64)
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("missing artwork did not finish loading")
	}
	view = ArtView{current: "loaded-cover", currentLoaded: true, ready: true}
	view.Layout(gtx, theme, cache, missing, image.Pt(64, 64), Shape{}, theme.Motion.Field)
	if view.previous != "" {
		t.Fatalf("failed artwork retained outgoing cover %q", view.previous)
	}
}

func TestArtViewPreservesOutgoingCoverAcrossRapidPendingChanges(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		http.Error(w, "missing", http.StatusNotFound)
	}))
	cache := clientart.New(clientart.Options{Workers: 1, HTTPClient: server.Client()})
	defer func() {
		close(release)
		cache.Close()
		server.Close()
	}()

	var operations op.Ops
	gtx := layout.Context{
		Ops:         &operations,
		Now:         time.Unix(2000, 0),
		Constraints: layout.Exact(image.Pt(64, 64)),
	}
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	view := ArtView{current: "loaded-cover", currentLoaded: true, ready: true}
	view.Layout(gtx, theme, cache, server.URL+"/c", image.Pt(64, 64), Shape{}, theme.Motion.Field)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("pending artwork request did not start")
	}
	view.Layout(gtx, theme, cache, server.URL+"/d", image.Pt(64, 64), Shape{}, theme.Motion.Field)
	if view.previous != "loaded-cover" {
		t.Fatalf("rapid pending change retained %q, want loaded-cover", view.previous)
	}
}
