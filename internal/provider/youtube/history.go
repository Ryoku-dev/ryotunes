package youtube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func playbackPing(response playerResponse, client string) model.PlaybackPing {
	return model.PlaybackPing{URL: response.PlaybackTracking.VideostatsPlaybackURL.BaseURL, Client: client}
}

// a resolve's ping is used only while it is fresh, so a report never replays an old player response
const (
	playbackPingLifetime = 15 * time.Minute
	maxRememberedPings   = 16
)

type rememberedPing struct {
	ping model.PlaybackPing
	at   time.Time
}

func (c *Client) rememberPlaybackPing(videoID string, ping model.PlaybackPing) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ping.URL == "" {
		delete(c.playbackPings, videoID)
		return
	}
	now := time.Now()
	if c.playbackPings == nil {
		c.playbackPings = make(map[string]rememberedPing)
	}
	if _, ok := c.playbackPings[videoID]; !ok && len(c.playbackPings) >= maxRememberedPings {
		oldest := ""
		for id, remembered := range c.playbackPings {
			if oldest == "" || remembered.at.Before(c.playbackPings[oldest].at) {
				oldest = id
			}
		}
		delete(c.playbackPings, oldest)
	}
	c.playbackPings[videoID] = rememberedPing{ping: ping, at: now}
}

// TakePlaybackPing removes and returns the ping from this track's latest resolve while it is fresh.
func (c *Client) TakePlaybackPing(videoID string) (model.PlaybackPing, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	remembered, ok := c.playbackPings[videoID]
	delete(c.playbackPings, videoID)
	if !ok || time.Since(remembered.at) >= playbackPingLifetime {
		return model.PlaybackPing{}, false
	}
	return remembered.ping, true
}

// PlaybackPing fetches watch-history metadata without resolving an audio stream.
func (c *Client) PlaybackPing(ctx context.Context, track model.Track) (model.PlaybackPing, error) {
	c.mu.RLock()
	signedIn := c.cookie != ""
	c.mu.RUnlock()
	if !signedIn {
		return model.PlaybackPing{}, nil
	}
	return c.fetchPlaybackPing(ctx, track)
}

func (c *Client) fetchPlaybackPing(ctx context.Context, track model.Track) (model.PlaybackPing, error) {
	order := ordinaryStreamClients
	if track.IsUpload {
		order = uploadStreamClients
	}
	c.mu.RLock()
	disabled := make(map[string]bool, len(c.disabled))
	for key, value := range c.disabled {
		disabled[key] = value
	}
	c.mu.RUnlock()
	var failures []error
	for _, key := range order {
		cfg := clients[string(key)]
		if disabled[string(key)] || cfg.poTokens {
			continue
		}
		response, err := c.player(ctx, cfg, track.VideoID)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		ping := playbackPing(response, cfg.key)
		if ping.URL != "" {
			return ping, nil
		}
		failures = append(failures, errors.New("player response carried no playback tracking URL"))
	}
	if len(failures) == 0 {
		return model.PlaybackPing{}, errors.New("no YouTube playback client available")
	}
	return model.PlaybackPing{}, errors.Join(failures...)
}

// ReportPlayback sends one authenticated YouTube watch-history ping.
func (c *Client) ReportPlayback(ctx context.Context, ping model.PlaybackPing, playlistID string) error {
	if ping.URL == "" {
		return nil
	}
	cfg, ok := clients[ping.Client]
	if !ok {
		return fmt.Errorf("unknown YouTube playback client %q", ping.Client)
	}
	c.mu.RLock()
	cookie, visitor := c.cookie, c.visitor
	c.mu.RUnlock()
	if cookie == "" {
		return nil
	}
	parsed, err := url.Parse(ping.URL)
	if err != nil {
		return fmt.Errorf("parse YouTube playback URL: %w", err)
	}
	// the URL comes from a player response or the cache, and it gets the session cookie
	if host := parsed.Hostname(); parsed.Scheme != "https" || host != "youtube.com" && !strings.HasSuffix(host, ".youtube.com") {
		return fmt.Errorf("refusing a YouTube playback report to %s://%s", parsed.Scheme, parsed.Hostname())
	}
	query := parsed.Query()
	query.Set("c", cfg.name)
	query.Set("cpn", playbackNonce())
	query.Set("ver", "2")
	if playlistID != "" {
		query.Set("list", playlistID)
		query.Set("referrer", playlistID)
	}
	parsed.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return fmt.Errorf("build YouTube playback report: %w", err)
	}
	setHeaders(req, cfg, visitor)
	req.Header.Set("Cookie", cookie)
	if sapisid := sapisidCookie(cookie); sapisid != "" {
		req.Header.Set("Authorization", SAPISIDHash(time.Now(), sapisid))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// a url.Error repeats the ping's tracking parameters in the log; the cause is enough
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("send YouTube playback report: %w", err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("read YouTube playback report: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("YouTube playback report: %s", resp.Status)
	}
	return nil
}
