package spotify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/devgianlu/go-librespot/session"
	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	providerapi "github.com/ryoku-dev/ryotunes/internal/provider"
)

// Options configures a Spotify provider.
type Options struct {
	DataDir    string
	Events     bus.Sink
	Log        *slog.Logger
	HTTPClient *http.Client
	// RestoreTimeout bounds opening the stored session; zero means 45 s.
	RestoreTimeout time.Duration
}

// DownloadSourceLabel is the honest source label for Spotify's YouTube search match.
const DownloadSourceLabel = "YouTube match for Spotify"

// DownloadQuery returns the yt-dlp search target used for a Spotify track.
func DownloadQuery(track model.Track) string {
	artists := strings.TrimSpace(track.Artists)
	title := strings.TrimSpace(track.Title)
	return "ytsearch1:" + strings.TrimSpace(artists+" "+title) + " official audio"
}

// Spotify is a signed-in Spotify Premium provider.
type Spotify struct {
	root             string
	credPath         string
	devicePath       string
	legacyCredPath   string
	legacyDevicePath string
	migratedPath     string
	hashPath         string
	events           bus.Sink
	log              *slog.Logger
	http             *http.Client
	liblog           *librespotLogger
	restoreTimeout   time.Duration

	mu          sync.RWMutex
	sess        *session.Session
	name        string
	premium     bool
	clientToken string
	closed      bool
	// unrecoverable is the dropped session whose one recovery attempt
	// failed; any installed session gets its own attempt.
	unrecoverable *session.Session

	authMu         sync.Mutex
	signing        bool
	authURL        string
	authCancel     context.CancelFunc
	authGeneration uint64

	restoreMu sync.Mutex

	hashMu sync.Mutex
	hashes *hashRegistry

	streamMu sync.RWMutex
	stream   *activeStream
	server   *http.Server
	baseURL  string
}

// New creates a Spotify provider and starts its private loopback stream server.
func New(opts Options) (*Spotify, error) {
	if opts.DataDir == "" {
		return nil, fmt.Errorf("spotify data directory is empty")
	}
	if opts.Events == nil {
		opts.Events = bus.Discard
	}
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = netx.Client(30 * time.Second)
	}
	if opts.RestoreTimeout <= 0 {
		opts.RestoreTimeout = 45 * time.Second
	}
	root := filepath.Join(opts.DataDir, "spotify")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create Spotify data directory: %w", err)
	}
	legacyRoot := filepath.Join(root, "spotify")
	p := &Spotify{
		root: root, credPath: filepath.Join(root, "credentials.json"),
		devicePath: filepath.Join(root, "device.id"), hashPath: filepath.Join(root, "pathfinder.json"),
		legacyCredPath: filepath.Join(legacyRoot, "credentials.json"), legacyDevicePath: filepath.Join(legacyRoot, "device.id"),
		migratedPath: filepath.Join(root, "1x-migrated"),
		events:       opts.Events, log: opts.Log, http: opts.HTTPClient, restoreTimeout: opts.RestoreTimeout,
	}
	p.liblog = &librespotLogger{log: opts.Log}
	if err := p.migrateLegacyCache(); err != nil {
		opts.Log.Warn("Spotify 1.x session was not fully migrated", "error", err)
	}
	if err := p.startStreamServer(); err != nil {
		return nil, err
	}
	return p, nil
}

// ID implements provider.Provider.
func (p *Spotify) ID() model.Provider { return model.ProviderSpotify }

// Capabilities implements provider.Provider.
func (p *Spotify) Capabilities() providerapi.Capabilities {
	return providerapi.Capabilities{
		Home: true, Search: true, Library: true, Playlists: true, Editing: true,
		Rate: true, SaveAlbum: true, Radio: true, Lyrics: true, Downloads: true, Account: true,
	}
}

// Status returns the current Spotify account and credential snapshot.
func (p *Spotify) Status() model.SpotifyStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	status := model.SpotifyStatus{SignedIn: p.sess != nil, Stored: fileExists(p.credPath)}
	if p.sess != nil {
		status.Name = new(p.name)
		status.Premium = new(p.premium)
	}
	return status
}

// Close tears down the Spotify session and loopback stream server.
func (p *Spotify) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	sess := p.sess
	p.sess = nil
	p.mu.Unlock()
	p.authMu.Lock()
	if p.authCancel != nil {
		p.authCancel()
	}
	p.authMu.Unlock()
	p.closeActiveStream()
	if sess != nil {
		sess.Close()
	}
	if p.server != nil {
		return p.server.Close()
	}
	return nil
}

func (p *Spotify) session(ctx context.Context) (*session.Session, error) {
	p.mu.RLock()
	sess := p.sess
	premium := p.premium
	p.mu.RUnlock()
	if sess == nil {
		return nil, providerapi.ErrSignInRequired
	}
	if !premium {
		return nil, providerapi.ErrNotPremium
	}
	select {
	case <-sess.Accesspoint().Done():
		return p.recoverSession(ctx, sess)
	default:
		return sess, nil
	}
}

func (p *Spotify) recoverSession(ctx context.Context, dead *session.Session) (*session.Session, error) {
	p.restoreMu.Lock()
	defer p.restoreMu.Unlock()
	p.mu.Lock()
	current := p.sess
	tried := p.unrecoverable == dead
	if current == dead && !tried {
		p.unrecoverable = dead
	}
	p.mu.Unlock()
	if current != dead {
		if current == nil {
			return nil, providerapi.ErrSignInRequired
		}
		return current, nil
	}
	if tried {
		return nil, fmt.Errorf("%w: Spotify session disconnected", providerapi.ErrSignInRequired)
	}
	p.authMu.Lock()
	generation := p.authGeneration
	p.authMu.Unlock()
	fresh, profile, err := p.openStoredSession(ctx)
	if err != nil {
		if transientFailure(err) {
			// After a resume the network is often not up yet; the next
			// request tries again instead of signing the session out.
			p.mu.Lock()
			if p.unrecoverable == dead {
				p.unrecoverable = nil
			}
			p.mu.Unlock()
			return nil, fmt.Errorf("%w: Spotify could not be reached", providerapi.ErrUnavailable)
		}
		return nil, fmt.Errorf("%w: %v", providerapi.ErrSignInRequired, err)
	}
	p.authMu.Lock()
	if generation != p.authGeneration {
		p.authMu.Unlock()
		fresh.Close()
		return nil, providerapi.ErrSignInRequired
	}
	if err := p.saveCredentials(fresh); err != nil {
		p.authMu.Unlock()
		fresh.Close()
		return nil, fmt.Errorf("%w: %v", providerapi.ErrSignInRequired, err)
	}
	installed := p.installSession(fresh, profile)
	p.authMu.Unlock()
	if !installed {
		return nil, providerapi.ErrSignInRequired
	}
	p.log.Info("recovered Spotify session from stored credentials")
	return fresh, nil
}

func (p *Spotify) installSession(sess *session.Session, profile accountProfile) bool {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		sess.Close()
		return false
	}
	old := p.sess
	p.sess = sess
	p.unrecoverable = nil
	p.name = profile.DisplayName
	p.premium = profile.Product == "premium" || profile.Product == ""
	p.mu.Unlock()
	if old != nil && old != sess {
		old.Close()
	}
	return true
}

func (p *Spotify) clearSession() *session.Session {
	p.mu.Lock()
	old := p.sess
	p.sess = nil
	p.unrecoverable = nil
	p.name = ""
	p.premium = false
	p.mu.Unlock()
	return old
}

func (p *Spotify) emitAuth(state string, url, name, message *string) {
	p.events.Emit(protocol.EventSpotifyAuth, model.SpotifyAuthEvent{State: state, URL: url, Name: name, Message: message})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func mapProviderError(err error) error {
	if err == nil || errors.Is(err, providerapi.ErrSignInRequired) || errors.Is(err, providerapi.ErrNotPremium) ||
		errors.Is(err, providerapi.ErrUnavailable) || errors.Is(err, providerapi.ErrNotFound) {
		return err
	}
	message := err.Error()
	switch {
	case containsAny(message, "401", "bad credentials", "invalid credentials", "authentication"):
		return fmt.Errorf("%w: %v", providerapi.ErrSignInRequired, err)
	case containsAny(message, "restricted", "unavailable", "403", "not available"):
		return fmt.Errorf("%w: %v", providerapi.ErrUnavailable, err)
	default:
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("%w: %v", providerapi.ErrUnavailable, err)
	}
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if len(needle) > 0 && len(s) >= len(needle) {
			for i := 0; i+len(needle) <= len(s); i++ {
				if equalFoldASCII(s[i:i+len(needle)], needle) {
					return true
				}
			}
		}
	}
	return false
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

var (
	_ providerapi.Provider       = (*Spotify)(nil)
	_ providerapi.Rater          = (*Spotify)(nil)
	_ providerapi.AlbumSaver     = (*Spotify)(nil)
	_ providerapi.PlaylistEditor = (*Spotify)(nil)
	_ providerapi.Lyricist       = (*Spotify)(nil)
	_ providerapi.Radio          = (*Spotify)(nil)
	_ providerapi.Grid           = (*Spotify)(nil)
)
