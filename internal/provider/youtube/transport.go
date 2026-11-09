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

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

const (
	apiBase        = "https://music.youtube.com/youtubei/v1/"
	musicOrigin    = "https://music.youtube.com"
	musicReferer   = "https://music.youtube.com/"
	visitorKey     = "visitor_data"
	cookieKey      = "session_cookie"
	dataSyncKey    = "data_sync_id"
	accountKey     = "account_json"
	selectionKey   = "selected_identity_json"
	pendingKey     = "account_selection_pending"
	lastVisitorKey = "last_visitor_try"
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
}

// Client is a YouTube Music provider and account surface.
type Client struct {
	kv      provider.KV
	http    *http.Client
	log     *slog.Logger
	dataDir string
	cookies CookieSource

	mu             sync.RWMutex
	cookie         string
	visitor        string
	dataSync       string
	account        model.Account
	proxy          string
	disabled       map[string]bool
	lastVisitorTry time.Time
	cipher         cipherState

	signInMu  sync.Mutex
	signingIn bool

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
	c := &Client{
		kv: o.KV, http: hc, log: o.Log, dataDir: o.DataDir, cookies: o.Cookies, proxy: o.Proxy,
		disabled: parseDisabled(o.DisabledStreamClients), ctx: clientCtx, cancel: cancel,
	}
	c.cookie, _ = o.KV.Get(cookieKey)
	c.visitor, _ = o.KV.Get(visitorKey)
	c.dataSync, _ = o.KV.Get(dataSyncKey)
	if raw, ok := o.KV.Get(accountKey); ok {
		_ = json.Unmarshal([]byte(raw), &c.account)
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
	c.mu.RLock()
	visitor, cookie, dataSync := c.visitor, c.cookie, c.dataSync
	c.mu.RUnlock()
	if override, ok := body["__dataSyncOverride"].(string); ok {
		dataSync = override
		delete(body, "__dataSyncOverride")
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
	endpointURL := apiBase + endpoint
	if strings.Contains(endpointURL, "?") {
		endpointURL += "&prettyPrint=false"
	} else {
		endpointURL += "?prettyPrint=false"
	}
	var last error
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
			if cookie != "" {
				return nil, errors.New("Your YouTube Music session expired, open the account menu and sign in again.")
			}
			return nil, fmt.Errorf("youtube: %s", resp.Status)
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
	return nil, last
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
