// Package lastfm implements desktop authorization, now-playing, and scrobbling.
package lastfm

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

const (
	apiRoot           = "https://ws.audioscrobbler.com/2.0/"
	authRoot          = "https://www.last.fm/api/auth/"
	sessionKeySetting = "lastfm_session_key"
	usernameSetting   = "lastfm_username"
)

// APIKey is injected at build time with -ldflags -X.
var APIKey string

// APISecret is injected at build time with -ldflags -X.
var APISecret string

// Events receives Last.fm state changes.
type Events interface {
	Emit(event string, data any)
}

// Options configures the Last.fm client.
type Options struct {
	Store      provider.KV
	Events     Events
	HTTPClient *http.Client
	OpenURL    func(string) error
	Log        *slog.Logger
}

// Client owns Last.fm authorization and scrobble state.
type Client struct {
	ctx     context.Context
	cancel  context.CancelFunc
	store   provider.KV
	events  Events
	http    *http.Client
	openURL func(string) error
	log     *slog.Logger

	mu             sync.Mutex
	authGeneration uint64
	sessionKey     string
	username       string
	track          *model.Track
	startedAt      int64
	duration       float64
	scrobbled      bool
	closed         bool
}

type apiError struct {
	code    int
	message string
}

func (e *apiError) Error() string { return e.message }

func (e *apiError) retryable() bool { return e.code == 0 || e.code == 14 || e.code == 16 }

// New creates a Last.fm client and restores any persisted session.
func New(ctx context.Context, options Options) (*Client, error) {
	if options.Store == nil {
		return nil, errors.New("lastfm: nil store")
	}
	if options.HTTPClient == nil {
		options.HTTPClient = netx.Client(15 * time.Second)
	}
	if options.OpenURL == nil {
		options.OpenURL = openURL
	}
	if options.Log == nil {
		options.Log = slog.Default()
	}
	clientContext, cancel := context.WithCancel(ctx)
	client := &Client{
		ctx:     clientContext,
		cancel:  cancel,
		store:   options.Store,
		events:  options.Events,
		http:    options.HTTPClient,
		openURL: options.OpenURL,
		log:     options.Log,
	}
	if APIKey != "" && APISecret != "" {
		if key, ok := options.Store.Get(sessionKeySetting); ok {
			client.sessionKey = strings.TrimSpace(key)
		}
		if username, ok := options.Store.Get(usernameSetting); ok && client.sessionKey != "" {
			client.username = strings.TrimSpace(username)
		}
	}
	return client, nil
}

// Connect starts desktop authorization and resolves through lastfm-state events.
func (c *Client) Connect(ctx context.Context) error {
	if APIKey == "" || APISecret == "" {
		return errors.New("Last.fm is not configured in this build.")
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("lastfm: client is closed")
	}
	c.authGeneration++
	generation := c.authGeneration
	c.mu.Unlock()

	body, err := c.call(ctx, "auth.getToken", nil, false)
	if err != nil {
		return fmt.Errorf("Last.fm: %s", apiErrorMessage(err))
	}
	token, _ := body["token"].(string)
	if token == "" {
		return errors.New("Last.fm returned no token")
	}
	if !c.authCurrent(generation) {
		return nil
	}
	authURL := authRoot + "?api_key=" + url.QueryEscape(APIKey) + "&token=" + url.QueryEscape(token)
	if err := c.openURL(authURL); err != nil {
		return err
	}
	go c.pollSession(generation, token)
	return nil
}

// Disconnect clears authorization, cancels pending polls, and emits disconnected state.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	c.authGeneration++
	c.sessionKey = ""
	c.username = ""
	c.mu.Unlock()
	firstErr := c.store.Delete(sessionKeySetting)
	if err := c.store.Delete(usernameSetting); firstErr == nil {
		firstErr = err
	}
	c.emitState(false, "", "")
	return firstErr
}

// Status returns the stable Last.fm connection snapshot.
func (c *Client) Status() model.LastfmStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	status := model.LastfmStatus{Configured: APIKey != "" && APISecret != "", Connected: c.sessionKey != ""}
	if status.Connected && c.username != "" {
		username := c.username
		status.Username = &username
	}
	return status
}

// Update supplies the current track and mpv position to the scrobbler.
func (c *Client) Update(now *model.Track, position, duration float64) {
	var nowPlaying *model.Track
	var scrobble *scrobbleRequest
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if !sameTrack(c.track, now) {
		if now == nil {
			c.track = nil
			c.duration = 0
			c.scrobbled = false
		} else {
			track := *now
			c.track = &track
			c.startedAt = time.Now().Unix()
			c.duration = max(duration, 0)
			c.scrobbled = false
			if c.sessionKey != "" && scrobbleable(&track) {
				nowPlaying = &track
			}
		}
	} else if c.track != nil {
		c.duration = max(duration, 0)
	}
	if c.track != nil && c.sessionKey != "" && !c.scrobbled && crossesThreshold(position, c.duration) {
		c.scrobbled = true
		if scrobbleable(c.track) {
			track := *c.track
			scrobble = &scrobbleRequest{track: track, sessionKey: c.sessionKey, startedAt: c.startedAt, duration: c.duration}
		}
	}
	sessionKey := c.sessionKey
	c.mu.Unlock()

	if nowPlaying != nil {
		go c.sendNowPlaying(sessionKey, *nowPlaying)
	}
	if scrobble != nil {
		go c.sendScrobble(*scrobble)
	}
}

// Close cancels pending authorization and prevents further scrobbles.
func (c *Client) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.authGeneration++
	c.mu.Unlock()
	c.cancel()
}

func (c *Client) pollSession(generation uint64, token string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range 60 {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
		}
		if !c.authCurrent(generation) {
			return
		}
		body, err := c.call(c.ctx, "auth.getSession", map[string]string{"token": token}, false)
		if err != nil {
			var apiErr *apiError
			if errors.As(err, &apiErr) && apiErr.retryable() {
				continue
			}
			c.emitState(false, "", "Last.fm: "+apiErrorMessage(err))
			return
		}
		session, ok := body["session"].(map[string]any)
		if !ok {
			c.emitState(false, "", "Last.fm sent a malformed session")
			return
		}
		name, nameOK := session["name"].(string)
		key, keyOK := session["key"].(string)
		if !nameOK || !keyOK || name == "" || key == "" {
			c.emitState(false, "", "Last.fm sent a malformed session")
			return
		}
		c.mu.Lock()
		if c.authGeneration != generation || c.closed {
			c.mu.Unlock()
			return
		}
		if err := c.store.Set(sessionKeySetting, key); err != nil {
			c.mu.Unlock()
			c.emitState(false, "", "Last.fm: "+err.Error())
			return
		}
		if err := c.store.Set(usernameSetting, name); err != nil {
			_ = c.store.Delete(sessionKeySetting)
			c.mu.Unlock()
			c.emitState(false, "", "Last.fm: "+err.Error())
			return
		}
		c.sessionKey = key
		c.username = name
		track := c.track
		c.mu.Unlock()
		c.log.Info("last.fm connected", "username", name)
		c.emitState(true, name, "")
		if track != nil && scrobbleable(track) {
			go c.sendNowPlaying(key, *track)
		}
		return
	}
	if c.authCurrent(generation) {
		c.emitState(false, "", "Last.fm authorization timed out, try again")
	}
}

func (c *Client) authCurrent(generation uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.authGeneration == generation
}

func (c *Client) emitState(connected bool, username, message string) {
	if c.events == nil {
		return
	}
	state := model.LastfmState{Configured: APIKey != "" && APISecret != "", Connected: connected}
	if username != "" {
		state.Username = &username
	}
	if message != "" {
		state.Error = &message
	}
	c.events.Emit(protocol.EventLastfmState, state)
}

func (c *Client) sendNowPlaying(sessionKey string, track model.Track) {
	params := map[string]string{"artist": track.Artists, "track": track.Title, "sk": sessionKey}
	if track.Album != nil && strings.TrimSpace(*track.Album) != "" {
		params["album"] = *track.Album
	}
	if _, err := c.call(c.ctx, "track.updateNowPlaying", params, true); err != nil {
		c.log.Debug("last.fm now playing failed", "error", err)
	}
}

type scrobbleRequest struct {
	track      model.Track
	sessionKey string
	startedAt  int64
	duration   float64
}

func (c *Client) sendScrobble(request scrobbleRequest) {
	params := map[string]string{
		"artist":    request.track.Artists,
		"track":     request.track.Title,
		"timestamp": strconv.FormatInt(request.startedAt, 10),
		"sk":        request.sessionKey,
	}
	if request.track.Album != nil && strings.TrimSpace(*request.track.Album) != "" {
		params["album"] = *request.track.Album
	}
	if request.duration > 0 {
		params["duration"] = strconv.FormatInt(int64(request.duration), 10)
	}
	if _, err := c.call(c.ctx, "track.scrobble", params, true); err != nil {
		c.log.Warn("last.fm scrobble failed", "error", err)
		return
	}
	c.log.Info("scrobbled to last.fm")
}

func (c *Client) call(ctx context.Context, method string, specific map[string]string, post bool) (map[string]any, error) {
	params := make(map[string]string, len(specific)+4)
	for key, value := range specific {
		params[key] = value
	}
	params["api_key"] = APIKey
	params["method"] = method
	params["api_sig"] = sign(params, APISecret)
	params["format"] = "json"
	values := make(url.Values, len(params))
	for key, value := range params {
		values.Set(key, value)
	}
	var request *http.Request
	var err error
	if post {
		request, err = http.NewRequestWithContext(ctx, http.MethodPost, apiRoot, strings.NewReader(values.Encode()))
		if err == nil {
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		request, err = http.NewRequestWithContext(ctx, http.MethodGet, apiRoot+"?"+values.Encode(), nil)
	}
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, &apiError{message: err.Error()}
	}
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return nil, &apiError{message: err.Error()}
	}
	if rawCode, ok := body["error"].(float64); ok {
		message, _ := body["message"].(string)
		if message == "" {
			message = "unknown last.fm error"
		}
		return nil, &apiError{code: int(rawCode), message: message}
	}
	return body, nil
}

func sign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if key != "format" && key != "api_sig" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var input strings.Builder
	for _, key := range keys {
		input.WriteString(key)
		input.WriteString(params[key])
	}
	input.WriteString(secret)
	digest := md5.Sum([]byte(input.String()))
	return hex.EncodeToString(digest[:])
}

func sameTrack(left, right *model.Track) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.VideoID != right.VideoID || left.Title != right.Title || left.Artists != right.Artists {
		return false
	}
	if left.Album == nil || right.Album == nil {
		return left.Album == right.Album
	}
	return *left.Album == *right.Album
}

func scrobbleable(track *model.Track) bool {
	return strings.TrimSpace(track.Title) != "" && strings.TrimSpace(track.Artists) != "" && !strings.EqualFold(strings.TrimSpace(track.Artists), "Unknown artist")
}

func crossesThreshold(position, duration float64) bool {
	if duration > 0 && duration < 30 {
		return false
	}
	threshold := 240.0
	if duration > 0 && duration/2 < threshold {
		threshold = duration / 2
	}
	return position >= threshold
}

func apiErrorMessage(err error) string {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return apiErr.message
	}
	return err.Error()
}

func openURL(raw string) error {
	if err := exec.Command("xdg-open", raw).Start(); err != nil {
		return fmt.Errorf("Couldn't open the browser: %w", err)
	}
	return nil
}
