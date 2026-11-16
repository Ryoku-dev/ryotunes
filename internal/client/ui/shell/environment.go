package shell

import (
	"context"
	"image"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// noticeLog runs the window's notices as the Hub's placards (HUB 6.8): a notice the
// listener can take back carries Undo, and stacks with the others (the placard keeps
// three), so no Undo is lost to a later one; the same plain notice again while it still
// shows replaces it instead of stacking a copy.
type noticeLog struct {
	undo     map[int]func()
	lastText string
	lastID   int
	lastAt   time.Time
	// lastUndo reports whether the last notice carries an Undo
	lastUndo bool
}

// push shows text, with Undo when undo is set, on placards at now.
func (log *noticeLog) push(placards *kit.Placard, now time.Time, text string, undo func()) {
	if text == "" {
		return
	}
	if undo == nil && !log.lastUndo && text == log.lastText && now.Sub(log.lastAt) < tokens.MotionPlacardLife {
		placards.Dismiss(log.lastID)
	}
	action := ""
	if undo != nil {
		action = "Undo"
	}
	id := placards.Push(now, text, action)
	log.lastText, log.lastID, log.lastAt, log.lastUndo = text, id, now, undo != nil
	if undo == nil {
		return
	}
	if log.undo == nil {
		log.undo = make(map[int]func())
	}
	log.undo[id] = undo
	// the placard keeps a few notices; an older one's Undo can no longer be pressed
	for older := range log.undo {
		if older <= id-tokens.SizePlacardStackCount {
			delete(log.undo, older)
		}
	}
}

// act runs the Undo of a notice pressed this frame.
func (log *noticeLog) act(gtx layout.Context, placards *kit.Placard) {
	for {
		id, ok := placards.Action(gtx)
		if !ok {
			return
		}
		if undo := log.undo[id]; undo != nil {
			delete(log.undo, id)
			undo()
		}
		if id == log.lastID {
			log.lastText = ""
		}
	}
}

// noticeText writes a notice in the Hub's voice: trimmed, with a capital first letter,
// and an error as a whole sentence ending in a period.
func noticeText(message string, kind env.ToastKind) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	first, size := utf8.DecodeRuneInString(message)
	if unicode.IsLower(first) {
		message = string(unicode.ToUpper(first)) + message[size:]
	}
	if kind == env.ToastError {
		last, _ := utf8.DecodeLastRuneInString(message)
		if !strings.ContainsRune(".!?…", last) {
			message += "."
		}
	}
	return message
}

func (s *uiShell) environment(now time.Time, theme *look.Theme) *env.Env {
	snapshot := s.runtime.store.Snapshot()
	lowResource := strings.EqualFold(strings.TrimSpace(snapshot.Settings[string(model.SettingLowResourceMode)]), "true")
	motion := !anim.Reduced() && !lowResource
	style := snapshot.Client.Style()
	spectrumInstalled := s.runtime.spectrum != nil && s.runtime.spectrum.Installed()
	scopeInstalled := s.runtime.scope != nil && s.runtime.scope.Installed()
	visualizerInstalled := spectrumInstalled
	if style == state.VisualizerOscilloscope || style == state.VisualizerVectorscope {
		visualizerInstalled = scopeInstalled
	}
	fx := env.Fx{
		Visualizer: snapshot.Client.VisualizerOn() && motion && visualizerInstalled,
		Wave:       snapshot.Client.WaveOn() && motion,
		Style:      style,
	}
	playing := snapshot.Playback.Now != nil && !snapshot.Playback.Paused
	spectrumStyle := style == state.VisualizerBars || style == state.VisualizerCurve
	if s.runtime.spectrum != nil {
		s.runtime.spectrum.SetActive(playing && fx.Visualizer && spectrumStyle)
	}
	if s.runtime.scope != nil {
		s.runtime.scope.SetActive(playing && fx.Visualizer && !spectrumStyle)
	}
	e := &env.Env{
		Th: theme, Overlays: &s.overlays, Store: s.runtime.store, Conn: s.runtime.client,
		Nav: s.nav, Art: s.runtime.art, Spectrum: s.runtime.spectrum, Scope: s.runtime.scope, Fx: fx, Now: now,
	}
	e.Toast = func(message string, kind env.ToastKind) {
		text := noticeText(message, kind)
		s.posts.Post(func() { s.notices.push(&s.placards, time.Now(), text, nil) })
	}
	e.Undoable = func(message string, undo func()) {
		text := noticeText(message, env.ToastInfo)
		s.posts.Post(func() { s.notices.push(&s.placards, time.Now(), text, undo) })
	}
	e.Go = func(fn func(context.Context)) {
		if fn == nil {
			return
		}
		go func() {
			fn(s.runtime.ctx)
			s.runtime.hub.Invalidate()
		}()
	}
	e.Confirm = func(title, body, action, required string, run func()) {
		s.posts.Post(func() {
			s.confirmReq = &confirmRequest{title: title, body: body, action: action, required: required, run: run}
			s.confirm.Dialog.Open = true
		})
	}
	e.Menu = func(target env.MenuTarget) {
		s.posts.Post(func() {
			if target.Context == "add-to-playlist" && target.Track != nil {
				s.itemMenu.OpenAdd(*target.Track)
				return
			}
			s.itemMenu.OpenAt(e, target, e.Overlays.Press())
		})
	}
	s.nav.onSound = func(anchor image.Rectangle) {
		e.Overlays.Open(&s.soundTag, anchor, look.Placement{
			Prefer: look.Above, Align: look.End, Width: 340,
		}, func(gtx layout.Context) layout.Dimensions {
			return s.sound.Layout(gtx, e)
		})
	}
	act.OpenAuthURLs(e)
	return e
}
