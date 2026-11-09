// Package lyrics resolves local and remote lyrics, parses timed formats, and caches source documents.
package lyrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/lyrics/romanize"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

const missTTL = 24 * time.Hour

// Request is the authoritative get_lyrics request shape.
type Request = protocol.LyricsParams

// Cache persists source lyrics and negative lookups.
type Cache interface {
	// GetLyrics reads a permanent hit or an unexpired negative lookup.
	GetLyrics(videoID string, now time.Time, missTTL time.Duration) (lyrics *model.Lyrics, hit bool, err error)
	// PutLyrics stores a source document or a nil negative lookup.
	PutLyrics(videoID string, lyrics *model.Lyrics, fetchedAt time.Time) error
}

// Options supplies the service's persistence, providers, and process seams.
type Options struct {
	// Cache persists source documents and misses.
	Cache Cache
	// Settings supplies lyrics_boidu and lyrics_romanize.
	Settings provider.KV
	// YouTube supplies YouTube Music timed and plain browse lyrics.
	YouTube provider.Lyricist
	// Spotify supplies Spotify colour lyrics.
	Spotify provider.Lyricist
	// HTTPClient performs public catalogue requests.
	HTTPClient *http.Client
	// Log receives repairable provider and cache failures.
	Log *slog.Logger
	// Version identifies Ryotunes in external API requests.
	Version string
	// Now supplies cache timestamps.
	Now func() time.Time
	// Romanize enables output-only readings independently of Settings.
	Romanize bool
	// LocalPath resolves a LOCAL track id to its audio path.
	LocalPath func(videoID string) (string, bool)
}

// Service resolves and caches lyrics.
type Service struct {
	cache     Cache
	settings  provider.KV
	youtube   provider.Lyricist
	spotify   provider.Lyricist
	http      *http.Client
	log       *slog.Logger
	userAgent string
	now       func() time.Time
	romanize  bool
	localPath func(string) (string, bool)

	mu      sync.Mutex
	flights map[string]*flight
}

type flight struct {
	done   chan struct{}
	lyrics *model.Lyrics
	err    error
}

type fetchOutcome struct {
	lyrics     *model.Lyrics
	definitive bool
}

// New constructs a lyrics service.
func New(o Options) *Service {
	client := o.HTTPClient
	if client == nil {
		client = netx.Client(0)
	}
	log := o.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	version := strings.TrimSpace(o.Version)
	if version == "" {
		version = "dev"
	}
	localPath := o.LocalPath
	if localPath == nil {
		localPath = pathFromLocalID
	}
	return &Service{
		cache: o.Cache, settings: o.Settings, youtube: o.YouTube, spotify: o.Spotify,
		http: client, log: log, userAgent: fmt.Sprintf("Ryotunes/%s desktop", version),
		now: now, romanize: o.Romanize, localPath: localPath, flights: make(map[string]*flight),
	}
}

// Get resolves lyrics for req, deduplicating concurrent lookups by track id.
func (s *Service) Get(ctx context.Context, req Request) (*model.Lyrics, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.VideoID == "" {
		return nil, nil
	}
	if model.ProviderOf(req.VideoID) == model.ProviderRadio {
		return nil, nil
	}

	f, leader := s.beginFlight(req.VideoID)
	if !leader {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-f.done:
			return s.forOutput(f.lyrics), f.err
		}
	}

	lyrics, err := s.resolve(ctx, req)
	s.endFlight(req.VideoID, f, lyrics, err)
	return s.forOutput(lyrics), err
}

func (s *Service) resolve(ctx context.Context, req Request) (*model.Lyrics, error) {
	if path, ok := s.localPath(req.VideoID); ok {
		if found, err := localLyrics(path); err == nil && found != nil {
			return found, nil
		} else if err != nil {
			s.log.Debug("local lyrics lookup failed", "path", path, "error", err)
		}
	}

	forced := strings.TrimSpace(os.Getenv("RYOTUNES_LYRICS_ONLY"))
	now := s.now()
	if forced == "" && s.cache != nil {
		lyrics, hit, err := s.cache.GetLyrics(req.VideoID, now, missTTL)
		if err != nil {
			s.log.Warn("lyrics cache read failed", "error", err)
		} else if hit {
			return sourceLyrics(lyrics), nil
		}
	}

	out, err := s.fetch(ctx, req, forced)
	clearRomanization(out.lyrics)
	if forced == "" && s.cache != nil && req.Duration != nil && *req.Duration > 0 && out.definitive {
		if putErr := s.cache.PutLyrics(req.VideoID, out.lyrics, now); putErr != nil {
			s.log.Warn("lyrics cache write failed", "error", putErr)
		}
	}
	return out.lyrics, err
}

func (s *Service) fetch(ctx context.Context, req Request, forced string) (fetchOutcome, error) {
	if forced != "" {
		var out fetchOutcome
		var err error
		switch forced {
		case "boidu":
			out, err = s.boidu(ctx, req)
		case "netease":
			out, err = s.netease(ctx, req)
		case "qq":
			out, err = s.qqMusic(ctx, req)
		case "kugou":
			out, err = s.kugou(ctx, req)
		default:
			s.log.Warn("RYOTUNES_LYRICS_ONLY: unknown provider", "provider", forced)
			return fetchOutcome{}, nil
		}
		return out, err
	}

	track := trackFromRequest(req)
	if model.ProviderOf(req.VideoID) == model.ProviderSpotify {
		if s.spotify == nil {
			return fetchOutcome{}, nil
		}
		lyrics, err := s.spotify.Lyrics(ctx, track)
		if errors.Is(err, provider.ErrNotFound) || errors.Is(err, provider.ErrUnsupported) {
			return fetchOutcome{definitive: true}, nil
		}
		return fetchOutcome{lyrics: lyrics, definitive: err == nil}, err
	}

	var errs []error
	definitive := false
	boiduEnabled := true
	if s.settings != nil {
		if raw, ok := s.settings.Get(string(model.SettingLyricsBoidu)); ok && raw == "false" {
			boiduEnabled = false
		}
	}
	if boiduEnabled {
		if out, err := s.boidu(ctx, req); err != nil {
			errs = append(errs, fmt.Errorf("boidu: %w", err))
		} else {
			definitive = out.definitive
			if out.lyrics != nil {
				return out, nil
			}
		}
	}

	exact, err := s.lrclibExact(ctx, req)
	if err != nil {
		errs = append(errs, fmt.Errorf("LRCLIB exact: %w", err))
	} else {
		definitive = true
		if exact != nil {
			if lyrics := exact.best(); lyrics != nil && (lyrics.Synced || lyrics.Instrumental) {
				return fetchOutcome{lyrics: lyrics, definitive: true}, nil
			}
		}
	}

	var youtubePlain *model.Lyrics
	if s.youtube != nil && model.ProviderOf(req.VideoID) == model.ProviderYouTube {
		lyrics, lyricErr := s.youtube.Lyrics(ctx, track)
		if errors.Is(lyricErr, provider.ErrNotFound) || errors.Is(lyricErr, provider.ErrUnsupported) {
			definitive = true
		} else if lyricErr != nil {
			errs = append(errs, fmt.Errorf("YouTube Music: %w", lyricErr))
		} else {
			definitive = true
			if lyrics != nil {
				if lyrics.Synced || lyrics.Instrumental {
					return fetchOutcome{lyrics: lyrics, definitive: true}, nil
				}
				youtubePlain = lyrics
			}
		}
	}

	for _, fetch := range []struct {
		name string
		fn   func(context.Context, Request) (fetchOutcome, error)
	}{
		{"Netease Cloud Music", s.netease},
		{"QQ Music", s.qqMusic},
		{"Kugou", s.kugou},
	} {
		out, fetchErr := fetch.fn(ctx, req)
		if fetchErr != nil {
			errs = append(errs, fmt.Errorf("%s: %w", fetch.name, fetchErr))
			continue
		}
		definitive = definitive || out.definitive
		if out.lyrics != nil {
			return fetchOutcome{lyrics: out.lyrics, definitive: definitive}, nil
		}
	}

	fuzzy, fuzzyErr := s.lrclibSearch(ctx, req)
	if fuzzyErr != nil {
		errs = append(errs, fmt.Errorf("LRCLIB search: %w", fuzzyErr))
	} else {
		definitive = true
		if fuzzy != nil {
			if lyrics := fuzzy.best(); lyrics != nil && lyrics.Synced {
				return fetchOutcome{lyrics: lyrics, definitive: true}, nil
			}
		}
	}
	if exact != nil {
		if lyrics := plainLyrics(exact.PlainLyrics, "LRCLIB"); lyrics != nil {
			return fetchOutcome{lyrics: lyrics, definitive: definitive}, nil
		}
	}
	if youtubePlain != nil {
		return fetchOutcome{lyrics: youtubePlain, definitive: true}, nil
	}
	if fuzzy != nil {
		if lyrics := fuzzy.best(); lyrics != nil {
			return fetchOutcome{lyrics: lyrics, definitive: definitive}, nil
		}
	}
	if !definitive && len(errs) != 0 {
		return fetchOutcome{}, errors.Join(errs...)
	}
	return fetchOutcome{definitive: definitive}, nil
}

func (s *Service) beginFlight(key string) (*flight, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.flights[key]; ok {
		return f, false
	}
	f := &flight{done: make(chan struct{})}
	s.flights[key] = f
	return f, true
}

func (s *Service) endFlight(key string, f *flight, lyrics *model.Lyrics, err error) {
	s.mu.Lock()
	f.lyrics = lyrics
	f.err = err
	delete(s.flights, key)
	close(f.done)
	s.mu.Unlock()
}

func (s *Service) forOutput(lyrics *model.Lyrics) *model.Lyrics {
	out := cloneLyrics(lyrics)
	if out == nil || !s.romanizationEnabled() || len(out.Lines) == 0 {
		return out
	}
	texts := make([]string, len(out.Lines))
	for i := range out.Lines {
		texts[i] = out.Lines[i].Text
	}
	joined := strings.Join(texts, "\n")
	reading, changed := romanize.Line(joined)
	if !changed {
		return out
	}
	romanized := strings.Split(reading, "\n")
	if len(romanized) != len(out.Lines) {
		return out
	}
	for i := range out.Lines {
		if !sameFoldedText(romanized[i], out.Lines[i].Text) && strings.TrimSpace(romanized[i]) != "" {
			value := romanized[i]
			out.Lines[i].Romanized = &value
		}
	}
	return out
}

func (s *Service) romanizationEnabled() bool {
	if s.romanize {
		return true
	}
	if s.settings == nil {
		return false
	}
	raw, ok := s.settings.Get(string(model.SettingLyricsRomanize))
	return ok && raw == "true"
}

func trackFromRequest(req Request) model.Track {
	track := model.Track{VideoID: req.VideoID, Title: req.Title, Artists: req.Artists, Album: req.Album}
	if req.Duration != nil && *req.Duration > 0 {
		duration := formatDuration(*req.Duration)
		track.Duration = &duration
	}
	return track
}

func formatDuration(seconds float64) string {
	total := int(seconds + 0.5)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

func cloneLyrics(in *model.Lyrics) *model.Lyrics {
	if in == nil {
		return nil
	}
	out := *in
	out.Lines = append([]model.LyricsLine(nil), in.Lines...)
	return &out
}

func sourceLyrics(in *model.Lyrics) *model.Lyrics {
	out := cloneLyrics(in)
	clearRomanization(out)
	return out
}

func clearRomanization(lyrics *model.Lyrics) {
	if lyrics == nil {
		return
	}
	for i := range lyrics.Lines {
		lyrics.Lines[i].Romanized = nil
	}
}
