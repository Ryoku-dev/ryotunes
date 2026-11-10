// Package youtube implements YouTube Music through the InnerTube JSON API.
package youtube

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
)

const (
	defaultAPIBase  = "https://music.youtube.com/youtubei/v1/"
	musicOrigin     = "https://music.youtube.com"
	musicReferer    = "https://music.youtube.com/"
	visitorKey      = "visitor_data"
	cookieKey       = "session_cookie"
	cookieSourceKey = "session_cookie_source"
	dataSyncKey     = "data_sync_id"
	accountKey      = "account_json"
	identityKey     = "account_identity"
	selectionKey    = "selected_identity_json"
	pendingKey      = "account_selection_pending"
	lastVisitorKey  = "last_visitor_try"
)

// Options configures a YouTube provider.
type Options struct {
	KV                    provider.KV
	HTTPClient            *http.Client
	Log                   *slog.Logger
	DataDir               string
	Proxy                 string
	DisabledStreamClients string
	Cookies               CookieSource
	// Bus receives auth-changed when the session is re-adopted or rejected.
	Bus bus.Sink
	// APIBase overrides the InnerTube endpoint base.
	APIBase string
}

// Client is a YouTube Music provider and account surface.
type Client struct {
	kv      provider.KV
	http    *http.Client
	log     *slog.Logger
	bus     bus.Sink
	apiBase string
	dataDir string
	cookies CookieSource

	mu             sync.RWMutex
	cookie         string
	cookieSource   *browsercookies.SessionSource
	visitor        string
	dataSync       string
	account        model.Account
	identity       string
	proxy          string
	disabled       map[string]bool
	lastVisitorTry time.Time
	cipher         cipherState
	playbackPings  map[string]rememberedPing

	signInMu  sync.Mutex
	signingIn bool
	refreshMu sync.Mutex
	// legacyMiss is guarded by refreshMu.
	legacyMiss *legacyMiss
	// commitMu serialises writes of the session keys; never held across
	// network. Order: refreshMu, then commitMu, then mu.
	commitMu sync.Mutex

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// New creates a YouTube Music provider and starts anonymous visitor bootstrap when needed.
func New(o Options) (*Client, error) {
	if o.KV == nil {
		return nil, errors.New("youtube: KV is required")
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.DataDir == "" {
		if cacheDir, err := os.UserCacheDir(); err == nil {
			o.DataDir = filepath.Join(cacheDir, "dev.ryoku.ryotunes")
		} else {
			o.DataDir = filepath.Join(os.TempDir(), fmt.Sprintf("ryotunes-%d", os.Getuid()))
		}
	}
	if o.Proxy == "" {
		o.Proxy, _ = o.KV.Get("proxy")
	}
	if o.DisabledStreamClients == "" {
		o.DisabledStreamClients, _ = o.KV.Get("disabled_stream_clients")
	}
	if err := netx.SetProxy(o.Proxy); err != nil {
		return nil, fmt.Errorf("youtube proxy: %w", err)
	}
	hc := o.HTTPClient
	if hc == nil {
		hc = netx.Client(60 * time.Second)
	}
	clientCtx, cancel := context.WithCancel(context.Background())
	if o.Cookies == nil {
		o.Cookies = systemCookies{}
	}
	if o.Bus == nil {
		o.Bus = bus.Discard
	}
	if o.APIBase == "" {
		o.APIBase = defaultAPIBase
	}
	c := &Client{
		kv: o.KV, http: hc, log: o.Log, bus: o.Bus, apiBase: strings.TrimRight(o.APIBase, "/") + "/",
		dataDir: o.DataDir, cookies: o.Cookies, proxy: o.Proxy,
		disabled: parseDisabled(o.DisabledStreamClients), playbackPings: make(map[string]rememberedPing),
		ctx: clientCtx, cancel: cancel,
	}
	c.cookie, _ = o.KV.Get(cookieKey)
	c.visitor, _ = o.KV.Get(visitorKey)
	c.dataSync, _ = o.KV.Get(dataSyncKey)
	c.identity, _ = o.KV.Get(identityKey)
	if raw, ok := o.KV.Get(accountKey); ok {
		_ = json.Unmarshal([]byte(raw), &c.account)
	}
	if raw, ok := o.KV.Get(cookieSourceKey); ok {
		var source browsercookies.SessionSource
		if json.Unmarshal([]byte(raw), &source) == nil && source.Path != "" {
			c.cookieSource = &source
		}
	}
	if raw, ok := o.KV.Get(lastVisitorKey); ok {
		if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
			c.lastVisitorTry = time.Unix(unix, 0)
		}
	}
	if c.visitor == "" {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.bootstrapVisitor(clientCtx)
		}()
	}
	return c, nil
}

func parseDisabled(value string) map[string]bool {
	if env, ok := os.LookupEnv("RYOTUNES_DISABLED_CLIENTS"); ok {
		value = env
	}
	result := make(map[string]bool)
	for _, part := range strings.Split(value, ",") {
		if key := strings.TrimSpace(part); key != "" {
			result[key] = true
		}
	}
	return result
}

// ID returns the YouTube provider identifier.
func (c *Client) ID() model.Provider { return model.ProviderYouTube }

// Capabilities reports every YouTube Music surface implemented by Client.
func (c *Client) Capabilities() provider.Capabilities {
	return provider.Capabilities{Home: true, Search: true, Suggest: true, Library: true, Playlists: true, Editing: true, Rate: true, SaveAlbum: true, Radio: true, Lyrics: true, Downloads: true, Account: true}
}

// GetStreamClients returns the fixed direct-player registry order.
func (c *Client) GetStreamClients() []model.StreamClient {
	return append([]model.StreamClient(nil), model.KnownStreamClients...)
}

// SetDisabledStreamClients updates the comma-separated resolver exclusion set.
func (c *Client) SetDisabledStreamClients(value string) {
	c.mu.Lock()
	c.disabled = parseDisabled(value)
	c.mu.Unlock()
}

// SetProxy updates the proxy used by the yt-dlp fallback.
func (c *Client) SetProxy(value string) {
	c.mu.Lock()
	c.proxy = value
	c.mu.Unlock()
}

// Close cancels and joins the anonymous visitor bootstrap.
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(c.cancel)
	c.wg.Wait()
}

func (c *Client) bootstrapVisitor(ctx context.Context) {
	for _, delay := range []time.Duration{0, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second} {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := c.fetchVisitorData(attemptCtx)
		cancel()
		if err == nil || ctx.Err() != nil {
			return
		}
		c.log.Debug("YouTube visitor bootstrap failed", "error", err)
	}
}

// RefreshVisitorData refreshes the anonymous browser identity with a five-minute cooldown.
func (c *Client) RefreshVisitorData(ctx context.Context) error {
	c.mu.Lock()
	if time.Since(c.lastVisitorTry) < 5*time.Minute {
		hasVisitor := c.visitor != ""
		c.mu.Unlock()
		if hasVisitor {
			return nil
		}
		return errors.New("visitorData not found in sw.js_data")
	}
	c.lastVisitorTry = time.Now()
	attempt := c.lastVisitorTry
	c.mu.Unlock()
	_ = c.kv.Set(lastVisitorKey, strconv.FormatInt(attempt.Unix(), 10))
	return c.fetchVisitorData(ctx)
}

func (c *Client) fetchVisitorData(ctx context.Context) error {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, musicOrigin+"/sw.js_data", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("sw.js_data: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	visitor, err := parseVisitorData(data)
	if err != nil {
		return err
	}
	if err := c.kv.Set(visitorKey, visitor); err != nil {
		return err
	}
	c.mu.Lock()
	c.visitor = visitor
	c.mu.Unlock()
	return nil
}

func parseVisitorData(data []byte) (string, error) {
	start := bytes.IndexByte(data, '[')
	if start < 0 {
		return "", errors.New("visitorData not found in sw.js_data")
	}
	var root []any
	if json.Unmarshal(data[start:], &root) != nil || len(root) == 0 {
		return "", errors.New("visitorData not found in sw.js_data")
	}
	row, ok := root[0].([]any)
	if !ok || len(row) < 3 {
		return "", errors.New("visitorData not found in sw.js_data")
	}
	values, ok := row[2].([]any)
	if !ok {
		return "", errors.New("visitorData not found in sw.js_data")
	}
	for _, value := range values {
		if s, ok := value.(string); ok && (strings.HasPrefix(s, "Cgt") || strings.HasPrefix(s, "Cgs")) {
			return s, nil
		}
	}
	return "", errors.New("visitorData not found in sw.js_data")
}

func (c *Client) do(ctx context.Context, clientKey, endpoint string, body map[string]any, authenticated bool) (map[string]any, error) {
	cfg, ok := clients[clientKey]
	if !ok {
		return nil, fmt.Errorf("unknown YouTube client %q", clientKey)
	}
	dataSyncOverride, hasDataSyncOverride := body["__dataSyncOverride"].(string)
	delete(body, "__dataSyncOverride")
	override, hasSessionOverride := ctx.Value(sessionOverrideKey{}).(sessionOverride)
	endpointURL := c.apiBase + endpoint
	if strings.Contains(endpointURL, "?") {
		endpointURL += "&prettyPrint=false"
	} else {
		endpointURL += "?prettyPrint=false"
	}
	for sessionAttempt := range 2 {
		c.mu.RLock()
		visitor, cookie, dataSync := c.visitor, c.cookie, c.dataSync
		c.mu.RUnlock()
		if hasSessionOverride {
			cookie = override.cookie
		}
		if hasDataSyncOverride {
			dataSync = dataSyncOverride
		}
		if cfg.loginRequired && cookie == "" {
			return nil, provider.ErrSignInRequired
		}
		videoID, _ := body["videoId"].(string)
		body["context"] = cfg.context(visitor, dataSync, cookie, videoID)
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		var last error
		retrySession := false
		for attempt := range 3 {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
				}
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(encoded))
			if err != nil {
				return nil, err
			}
			setHeaders(req, cfg, visitor)
			if authenticated && cookie != "" {
				req.Header.Set("Cookie", cookie)
				if cfg.loginSupported {
					if sapisid := sapisidCookie(cookie); sapisid != "" {
						req.Header.Set("Authorization", SAPISIDHash(time.Now(), sapisid))
					}
				}
			}
			resp, err := c.http.Do(req)
			if err != nil {
				last = err
				continue
			}
			data, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				last = readErr
				continue
			}
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				if cookie == "" {
					return nil, fmt.Errorf("youtube: %s", resp.Status)
				}
				if hasSessionOverride {
					return nil, sessionExpiredError(nil)
				}
				retry, err := c.handleRejection(ctx, cookie, dataSync, !hasDataSyncOverride, resp.Status, sessionAttempt == 0)
				if !retry {
					return nil, err
				}
				retrySession = true
				break
			}
			if resp.StatusCode/100 != 2 {
				last = fmt.Errorf("youtube: %s", resp.Status)
				if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
					return nil, last
				}
				continue
			}
			var result map[string]any
			if len(data) != 0 {
				if err := json.Unmarshal(data, &result); err != nil {
					return nil, err
				}
			}
			return result, nil
		}
		if retrySession {
			continue
		}
		return nil, last
	}
	return nil, sessionExpiredError(nil)
}

// handleRejection decides what a 401 or 403 means for the session that sent
// cookie. It first follows the browser; when that offers nothing newer it
// asks the account menu whether the cookie still works, because InnerTube
// also answers 403 for reasons that are not the session. retry reports that
// the request should be sent again with the session now in place.
func (c *Client) handleRejection(ctx context.Context, cookie, dataSync string, storedChannel bool, status string, mayRefresh bool) (retry bool, err error) {
	var refreshErr error
	if mayRefresh {
		changed, err := c.refreshFromSource(ctx, cookie)
		if err == nil && changed {
			return true, nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		refreshErr = err
	}
	account, _, _, verifyErr := c.fetchAccountWithCookie(ctx, cookie, dataSync)
	if verifyErr == nil && account.Name != nil {
		return false, fmt.Errorf("youtube: %s", status)
	}
	if verifyErr != nil && !definitiveSessionFailure(verifyErr) {
		if refreshErr != nil {
			return false, fmt.Errorf("refresh YouTube browser session: %w", errors.Join(refreshErr, verifyErr))
		}
		return false, verifyErr
	}
	if storedChannel && dataSync != "" {
		// the session may be fine and only the remembered channel refused
		account, _, visitor, plainErr := c.fetchAccountWithoutChannel(ctx, cookie)
		if plainErr == nil && account.Name != nil {
			if err := c.forgetChannel(cookie, visitor, account); err != nil {
				return false, err
			}
			if mayRefresh {
				return true, nil
			}
			return false, fmt.Errorf("youtube: %s", status)
		}
	}
	forgot, err := c.markSessionRejected(cookie)
	if err != nil {
		return false, err
	}
	if !forgot && mayRefresh {
		c.mu.RLock()
		replaced := c.cookie != "" && c.cookie != cookie
		c.mu.RUnlock()
		if replaced {
			return true, nil
		}
	}
	return false, sessionExpiredError(refreshErr)
}

// forgetChannel drops a remembered channel YouTube no longer accepts for a
// session it still accepts, so requests go to the account itself.
func (c *Client) forgetChannel(cookie, visitor string, account model.Account) error {
	c.commitMu.Lock()
	defer c.commitMu.Unlock()
	c.mu.RLock()
	current, canSwitch := c.cookie, c.account.CanSwitch
	c.mu.RUnlock()
	if current != cookie {
		return nil
	}
	account.CanSwitch = canSwitch
	if err := c.persistAccount(cookie, "", visitor, account); err != nil {
		return err
	}
	if err := c.kv.Delete(selectionKey); err != nil {
		return err
	}
	c.emitAccount(account)
	return nil
}

func setHeaders(req *http.Request, cfg clientConfig, visitor string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("X-Goog-Api-Format-Version", "1")
	req.Header.Set("X-YouTube-Client-Name", cfg.id)
	req.Header.Set("X-YouTube-Client-Version", cfg.version)
	req.Header.Set("X-Origin", musicOrigin)
	req.Header.Set("Origin", musicOrigin)
	req.Header.Set("Referer", musicReferer)
	req.Header.Set("User-Agent", cfg.userAgent)
	if visitor != "" {
		req.Header.Set("X-Goog-Visitor-Id", visitor)
	}
}

// SAPISIDHash computes YouTube's exact authenticated request header.
func SAPISIDHash(now time.Time, sapisid string) string {
	epoch := fmt.Sprintf("%d", now.Unix())
	hash := sha1.Sum([]byte(epoch + " " + sapisid + " " + musicOrigin))
	return "SAPISIDHASH " + epoch + "_" + hex.EncodeToString(hash[:])
}

func sapisidCookie(header string) string {
	cookies := parseCookieHeader(header)
	if value := cookies["SAPISID"]; value != "" {
		return value
	}
	return cookies["__Secure-3PAPISID"]
}

func parseCookieHeader(header string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name != "" {
			result[name] = value
		}
	}
	return result
}

func localeLanguage() string {
	locale := localeValue()
	if locale == "" || locale == "C" || locale == "POSIX" {
		return "en"
	}
	locale = strings.Split(locale, ".")[0]
	locale = strings.Split(locale, "_")[0]
	return strings.ToLower(locale)
}

func localeRegion() string {
	locale := localeValue()
	locale = strings.Split(locale, ".")[0]
	parts := strings.Split(locale, "_")
	if len(parts) < 2 || len(parts[1]) != 2 {
		return "US"
	}
	return strings.ToUpper(parts[1])
}

func localeValue() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func (c *Client) cachePath(parts ...string) string {
	return filepath.Join(append([]string{c.dataDir}, parts...)...)
}
