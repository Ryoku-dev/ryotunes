// Package daemon wires the parts of ryotunesd together and answers the wire
// protocol. Handlers are thin: decode params, call the owning package, emit.
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/diag"
	"github.com/ryoku-dev/ryotunes/internal/downloads"
	"github.com/ryoku-dev/ryotunes/internal/integrations/discord"
	"github.com/ryoku-dev/ryotunes/internal/integrations/lastfm"
	"github.com/ryoku-dev/ryotunes/internal/integrations/listentogether"
	"github.com/ryoku-dev/ryotunes/internal/integrations/mpris"
	"github.com/ryoku-dev/ryotunes/internal/integrations/tray"
	"github.com/ryoku-dev/ryotunes/internal/lyrics"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/player"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/local"
	"github.com/ryoku-dev/ryotunes/internal/provider/radio"
	"github.com/ryoku-dev/ryotunes/internal/provider/soundcloud"
	"github.com/ryoku-dev/ryotunes/internal/provider/spotify"
	"github.com/ryoku-dev/ryotunes/internal/provider/tidal"
	"github.com/ryoku-dev/ryotunes/internal/provider/youtube"
	"github.com/ryoku-dev/ryotunes/internal/queue"
	"github.com/ryoku-dev/ryotunes/internal/store"
)

// App is the running daemon. New builds it; Run serves it. Optional parts
// (MPRIS, tray, Discord) are nil when their bus or socket is unavailable,
// and every handler checks before use.
type App struct {
	Log   *slog.Logger
	Diag  *diag.Handler
	Dirs  store.Dirs
	Store *store.Store
	Bus   *bus.Fanout

	Player        *player.Player
	Queue         *queue.Queue
	Providers     *provider.Registry
	EngineVersion string

	YouTube    *youtube.Client
	Spotify    *spotify.Spotify
	SoundCloud *soundcloud.Provider
	Tidal      *tidal.Provider
	Local      *local.Provider
	Radio      *radio.Provider

	Lyrics    *lyrics.Service
	Downloads *downloads.Manager
	LT        *listentogether.Client
	Lastfm    *lastfm.Client
	Discord   *discord.Client
	MPRIS     *mpris.Service
	Tray      *tray.Tray

	Server *Server

	// ctx is cancelled on quit; long work started by handlers derives from it.
	ctx    context.Context
	cancel context.CancelFunc
	// background joins startup restoration and scans before durable state closes.
	background sync.WaitGroup

	mu       sync.Mutex
	provider string // the selected catalogue provider

	equalizerMu             sync.Mutex
	equalizerPlayer         equalizerPlayer
	equalizerStore          provider.KV
	equalizerPresets        []model.EQPreset
	equalizerActiveBlocked  bool
	equalizerPresetsBlocked bool
}

// Handler answers one method. params is the raw JSON (null when absent).
// The result is marshalled as the response; a nil result is JSON null.
type Handler func(ctx context.Context, a *App, params json.RawMessage) (any, error)

var handlers = map[string]Handler{}

// register adds a handler at init time; a duplicate is a wiring bug.
func register(name string, h Handler) {
	if _, dup := handlers[name]; dup {
		panic("daemon: method registered twice: " + name)
	}
	if _, ok := protocol.Lookup(name); !ok {
		panic("daemon: handler for a method the protocol does not declare: " + name)
	}
	handlers[name] = h
}

// Dispatch runs one request and returns its result or a protocol error.
func (a *App) Dispatch(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error) {
	h, ok := handlers[method]
	if !ok {
		return nil, &protocol.Error{Code: protocol.ErrorUnknownMethod, Message: method}
	}
	result, err := h(ctx, a, params)
	if err != nil {
		rpcError := asProtocolError(err)
		_, generic := userMessageMapping(err)
		if rpcError.Code == protocol.ErrorFailed && generic && a.Log != nil {
			a.Log.Warn("daemon request failed", "method", method, "error", err)
		}
		return nil, rpcError
	}
	return result, nil
}

// paramsError marks a params decoding failure so it maps to bad_params.
type paramsError struct{ err error }

func (e paramsError) Error() string { return e.err.Error() }
func (e paramsError) Unwrap() error { return e.err }

// decode unmarshals params into P; null or absent params leave P zero.
func decode[P any](raw json.RawMessage) (P, error) {
	var p P
	if len(raw) == 0 || string(raw) == "null" {
		return p, nil
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, paramsError{err}
	}
	return p, nil
}

// typed adapts a typed function to a Handler.
func typed[P, R any](fn func(ctx context.Context, a *App, p P) (R, error)) Handler {
	return func(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
		p, err := decode[P](raw)
		if err != nil {
			return nil, err
		}
		return fn(ctx, a, p)
	}
}

// codedError carries an explicit protocol code (spotify_signed_out, client_side...).
type codedError struct {
	code    protocol.ErrorCode
	message string
}

func (e codedError) Error() string { return e.message }

// coded returns an error that maps to the given protocol code.
func coded(code protocol.ErrorCode, format string, args ...any) error {
	return codedError{code: code, message: fmt.Sprintf(format, args...)}
}

func asProtocolError(err error) *protocol.Error {
	var pe paramsError
	if errors.As(err, &pe) {
		return &protocol.Error{Code: protocol.ErrorBadParams, Message: pe.Error()}
	}
	var ce codedError
	if errors.As(err, &ce) {
		return &protocol.Error{Code: ce.code, Message: ce.message}
	}
	return &protocol.Error{Code: protocol.ErrorFailed, Message: userMessage(err)}
}
