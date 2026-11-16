package shell

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
)

func TestNoticeTextFollowsTheHubsVoice(t *testing.T) {
	for _, test := range []struct {
		message string
		kind    env.ToastKind
		want    string
	}{
		{"  dial unix: no such file  ", env.ToastError, "Dial unix: no such file."},
		{"The daemon is not answering.", env.ToastError, "The daemon is not answering."},
		{"added to queue", env.ToastInfo, "Added to queue"},
		{"", env.ToastError, ""},
	} {
		if got := noticeText(test.message, test.kind); got != test.want {
			t.Fatalf("noticeText(%q, %s) = %q, want %q", test.message, test.kind, got, test.want)
		}
	}
}

func TestUndoableNoticesStackAndKeepEveryUndo(t *testing.T) {
	var placards kit.Placard
	var log noticeLog
	now := time.Unix(100, 0)
	undone := 0
	ids := make([]int, 0, 4)
	for step := range 4 {
		log.push(&placards, now.Add(time.Duration(step)*time.Second), "Removed from liked songs.", func() { undone += 1 << step })
		ids = append(ids, log.lastID)
	}
	// the placard keeps three: the oldest Undo goes, the three showing stay
	if _, kept := log.undo[ids[0]]; kept {
		t.Fatal("an Undo past the placard's three was kept")
	}
	for _, id := range ids[1:] {
		if log.undo[id] == nil {
			t.Fatalf("a repeated undoable notice dropped the Undo of notice %d", id)
		}
	}
	// each Undo runs its own action
	log.undo[ids[1]]()
	log.undo[ids[3]]()
	if undone != 1<<1|1<<3 {
		t.Fatalf("the Undos ran %b, want their own actions", undone)
	}
	// a plain notice repeating an undoable one stacks; it never takes the Undo away
	log.push(&placards, now.Add(4*time.Second), "Removed from liked songs.", nil)
	if log.undo[ids[3]] == nil {
		t.Fatal("a plain repeat dropped the undoable notice's Undo")
	}
}

func TestPlainNoticesCoalesce(t *testing.T) {
	var placards kit.Placard
	var log noticeLog
	now := time.Unix(100, 0)
	log.push(&placards, now, "The daemon is not answering.", nil)
	first := log.lastID
	log.push(&placards, now.Add(time.Second), "The daemon is not answering.", nil)
	if log.lastID == first || log.lastUndo {
		t.Fatal("a plain repeat did not replace the one showing")
	}
	if len(log.undo) != 0 {
		t.Fatalf("plain notices kept %d Undo actions", len(log.undo))
	}
}

// fakePage is a page with no masthead of its own.
type fakePage struct{}

func (fakePage) Layout(gtx layout.Context, _ *env.Env) layout.Dimensions {
	return layout.Dimensions{Size: gtx.Constraints.Max}
}
func (fakePage) Title() string { return "" }

type partPage struct{ fakePage }

func (partPage) Masthead() (string, string, float32) { return "Songs", "曲", .25 }

type numberedPage struct{ fakePage }

func (numberedPage) Masthead() (string, string, string, float32) { return "05.3", "Sound", "音", .5 }

func TestPageSheetReadsThePagesPart(t *testing.T) {
	head, progress := pageSheet(env.Route{Kind: env.RouteAlbum}, fakePage{}, "YouTube Music")
	if head.Place.ID != sheet.Library.ID || head.Part != "Album" || head.Provider != "YouTube Music" || progress >= 0 {
		t.Fatalf("album sheet = %+v, %v; want the Library's Album, the provider, no progress", head, progress)
	}
	head, progress = pageSheet(env.Route{Kind: env.RouteLibrary}, partPage{}, "")
	if head.Part != "Songs" || head.PartGloss != "曲" || progress != .25 {
		t.Fatalf("library sheet = %+v, %v; want Songs 曲 at .25", head, progress)
	}
	head, progress = pageSheet(env.Route{Kind: env.RouteSettings}, numberedPage{}, "")
	if head.Place.Number != "05.3" || head.Place.ID != sheet.Settings.ID || head.Part != "Sound" || progress != .5 {
		t.Fatalf("settings sheet = %+v, %v; want § 05.3 SETTINGS / SOUND at .5", head, progress)
	}
	if head.Place.PagePlace() != sheet.Settings.PagePlace() {
		t.Fatal("renumbering the section lost the page's place on the sheet")
	}
}

func TestPageRiseSlidesInAndSettles(t *testing.T) {
	anim.SetReduce(false)
	defer anim.SetReduce(false)
	var operations op.Ops
	var router input.Router
	var shell uiShell
	now := time.Unix(100, 0)
	frame := func(key string, d time.Duration) bool {
		now = now.Add(d)
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Now: now, Source: router.Source(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 300)),
		}
		shell.pageRise.Layout(gtx, key, fakePage{}.Layout2)
		router.Frame(&operations)
		_, wake := router.WakeupTime()
		return wake
	}
	if frame("home", 0) {
		t.Fatal("the first page rose; it should stand")
	}
	if !frame("library", time.Millisecond) || !shell.pageRise.Active() {
		t.Fatal("a new page did not rise")
	}
	if !frame("library", 150*time.Millisecond) {
		t.Fatal("the rise stopped asking for frames mid-way")
	}
	frame("library", 400*time.Millisecond)
	if frame("library", time.Millisecond) {
		t.Fatal("a risen page kept asking for frames")
	}
	if shell.pageRise.Active() {
		t.Fatal("the rise is still active after 300 ms")
	}
}

// Layout2 is fakePage's widget with no environment.
func (fakePage) Layout2(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: gtx.Constraints.Max}
}
