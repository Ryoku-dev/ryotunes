// Package protocol owns the complete version-one Ryotunes NDJSON wire contract.
package protocol

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Version is the current Ryotunes wire protocol version.
const Version = 1

// MaxLineSize is the largest accepted NDJSON object, including its newline.
const MaxLineSize = 8 << 20

// ErrLineTooLong reports an NDJSON object larger than MaxLineSize.
var ErrLineTooLong = errors.New("protocol line exceeds maximum size")

// Request is one client RPC request.
type Request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Response is one successful or failed RPC response.
type Response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Event is one daemon broadcast to subscribed clients.
type Event struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// ErrorCode classifies a protocol error without parsing its message.
type ErrorCode string

const (
	// ErrorBadRequest is malformed request framing or JSON.
	ErrorBadRequest ErrorCode = "bad_request"
	// ErrorBadParams is a missing or incorrectly typed parameter.
	ErrorBadParams ErrorCode = "bad_params"
	// ErrorFailed is a backend or validation failure.
	ErrorFailed ErrorCode = "failed"
	// ErrorUnknownMethod is an unregistered method name.
	ErrorUnknownMethod ErrorCode = "unknown_method"
	// ErrorClientSide identifies a method intentionally handled by the UI.
	ErrorClientSide ErrorCode = "client_side"
	// ErrorDisconnected reports a lost daemon connection.
	ErrorDisconnected ErrorCode = "disconnected"
	// ErrorSpotifySignedOut asks the user to sign in to Spotify.
	ErrorSpotifySignedOut ErrorCode = "spotify_signed_out"
	// ErrorSpotify is a Spotify provider failure.
	ErrorSpotify ErrorCode = "spotify"
	// ErrorSoundCloud is a SoundCloud provider failure.
	ErrorSoundCloud ErrorCode = "soundcloud"
	// ErrorTidal is a TIDAL provider failure.
	ErrorTidal ErrorCode = "tidal"
)

// Error is a typed wire error.
type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// Error returns the wire error message.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ReadLine reads one bounded NDJSON object and removes its line terminator.
func ReadLine(r *bufio.Reader) ([]byte, error) {
	line := make([]byte, 0, 4096)
	for {
		fragment, err := r.ReadSlice('\n')
		if len(line)+len(fragment) > MaxLineSize {
			if !endsLine(fragment) {
				drainLine(r)
			}
			return nil, ErrLineTooLong
		}
		line = append(line, fragment...)
		switch {
		case err == nil:
			return trimLineEnd(line), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(line) != 0:
			return trimLineEnd(line), nil
		default:
			return nil, err
		}
	}
}

// Encoder writes one compact JSON object and newline per call.
type Encoder struct {
	writer  *bufio.Writer
	encoder *json.Encoder
}

// NewEncoder creates a compact NDJSON encoder.
func NewEncoder(w io.Writer) *Encoder {
	writer := bufio.NewWriter(w)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return &Encoder{writer: writer, encoder: encoder}
}

// Encode writes and flushes one compact NDJSON object.
func (e *Encoder) Encode(value any) error {
	if err := e.encoder.Encode(value); err != nil {
		return err
	}
	return e.writer.Flush()
}

// RuntimeDir returns the private per-user runtime directory.
func RuntimeDir() string {
	if base := os.Getenv("XDG_RUNTIME_DIR"); base != "" && filepath.IsAbs(base) {
		return filepath.Join(base, "ryotunes")
	}
	return filepath.Join("/tmp", fmt.Sprintf("ryotunes-%d", os.Getuid()))
}

// SocketPath returns the daemon's Unix socket path.
func SocketPath() string {
	return filepath.Join(RuntimeDir(), "ryotunesd.sock")
}

// LockPath returns the daemon singleton lock path.
func LockPath() string {
	return SocketPath() + ".lock"
}

func endsLine(fragment []byte) bool {
	return len(fragment) != 0 && fragment[len(fragment)-1] == '\n'
}

func drainLine(r *bufio.Reader) {
	for {
		fragment, err := r.ReadSlice('\n')
		if endsLine(fragment) || (err != nil && !errors.Is(err, bufio.ErrBufferFull)) {
			return
		}
	}
}

func trimLineEnd(line []byte) []byte {
	if len(line) != 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) != 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line
}
