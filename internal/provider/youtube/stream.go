package youtube

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

type playerFormat struct {
	Itag            int    `json:"itag"`
	URL             string `json:"url"`
	SignatureCipher string `json:"signatureCipher"`
	Cipher          string `json:"cipher"`
	MimeType        string `json:"mimeType"`
	Bitrate         int    `json:"bitrate"`
	AudioQuality    string `json:"audioQuality"`
	AudioChannels   int    `json:"audioChannels"`
	ContentLength   string `json:"contentLength"`
}

type playerResponse struct {
	PlayabilityStatus struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"playabilityStatus"`
	StreamingData struct {
		ExpiresInSeconds string         `json:"expiresInSeconds"`
		Formats          []playerFormat `json:"formats"`
		AdaptiveFormats  []playerFormat `json:"adaptiveFormats"`
	} `json:"streamingData"`
	PlayerConfig struct {
		AudioConfig struct {
			LoudnessDB           float64 `json:"loudnessDb"`
			PerceptualLoudnessDB float64 `json:"perceptualLoudnessDb"`
		} `json:"audioConfig"`
	} `json:"playerConfig"`
	VideoDetails struct {
		Title         string `json:"title"`
		LengthSeconds string `json:"lengthSeconds"`
	} `json:"videoDetails"`
	PlaybackTracking struct {
		VideostatsPlaybackURL struct {
			BaseURL string `json:"baseUrl"`
		} `json:"videostatsPlaybackUrl"`
	} `json:"playbackTracking"`
}

// Resolve returns a playable audio stream, falling back to yt-dlp after direct clients.
func (c *Client) Resolve(ctx context.Context, track model.Track, quality model.Quality) (model.Stream, error) {
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
	botGated := false
	var failures []error
	for _, key := range order {
		cfg := clients[string(key)]
		if disabled[string(key)] || cfg.poTokens {
			continue
		}
		stream, response, err := c.resolveClient(ctx, cfg, track.VideoID, quality)
		if err == nil {
			c.trackPlayback(response, "")
			return stream, nil
		}
		if errors.Is(err, provider.ErrBotGated) {
			botGated = true
		}
		failures = append(failures, err)
	}
	stream, err := c.resolveYTDLP(ctx, track.VideoID, quality)
	if err == nil {
		return stream, nil
	}
	failures = append(failures, err)
	if botGated {
		return model.Stream{}, fmt.Errorf("%w: YouTube's bot gate rejected every playback client for %s", provider.ErrBotGated, track.VideoID)
	}
	return model.Stream{}, errors.Join(failures...)
}

func (c *Client) resolveClient(ctx context.Context, cfg clientConfig, videoID string, quality model.Quality) (model.Stream, playerResponse, error) {
	body := map[string]any{"videoId": videoID, "contentCheckOk": true, "racyCheckOk": true}
	if cfg.signature {
		sts, err := c.signatureTimestamp(ctx)
		if err != nil {
			return model.Stream{}, playerResponse{}, err
		}
		if sts != 0 {
			body["playbackContext"] = map[string]any{"contentPlaybackContext": map[string]any{"signatureTimestamp": sts}}
		}
	}
	root, err := c.do(ctx, cfg.key, "player", body, true)
	if err != nil {
		return model.Stream{}, playerResponse{}, err
	}
	data, _ := json.Marshal(root)
	var response playerResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return model.Stream{}, response, err
	}
	if response.PlayabilityStatus.Status == "LOGIN_REQUIRED" && response.PlayabilityStatus.Reason == "Sign in to confirm you're not a bot" {
		return model.Stream{}, response, provider.ErrBotGated
	}
	if response.PlayabilityStatus.Status != "OK" {
		reason := response.PlayabilityStatus.Reason
		if reason == "" {
			reason = response.PlayabilityStatus.Status
		}
		return model.Stream{}, response, fmt.Errorf("%w: %s", provider.ErrUnavailable, reason)
	}
	formats := append(response.StreamingData.AdaptiveFormats, response.StreamingData.Formats...)
	selected, ok := chooseFormat(formats, quality)
	if !ok {
		return model.Stream{}, response, fmt.Errorf("%w: no audio stream", provider.ErrUnavailable)
	}
	streamURL := selected.URL
	if streamURL == "" {
		cipherText := selected.SignatureCipher
		if cipherText == "" {
			cipherText = selected.Cipher
		}
		streamURL, err = c.decipherURL(ctx, cipherText)
		if err != nil {
			return model.Stream{}, response, err
		}
	}
	streamURL, err = c.transformN(ctx, streamURL)
	if err != nil {
		return model.Stream{}, response, err
	}
	stream := model.Stream{URL: streamURL, Headers: map[string]string{"Origin": musicOrigin, "Referer": musicReferer}, UserAgent: cfg.userAgent, Codec: codecFromMIME(selected.MimeType), Bitrate: selected.Bitrate}
	if expire := expiryFromURL(streamURL); expire != nil {
		stream.ExpiresAt = expire
	} else if seconds, _ := strconv.ParseInt(response.StreamingData.ExpiresInSeconds, 10, 64); seconds > 0 {
		value := time.Now().Unix() + seconds
		stream.ExpiresAt = &value
	}
	loudness := response.PlayerConfig.AudioConfig.LoudnessDB
	if loudness != 0 {
		stream.LoudnessDB = &loudness
	}
	return stream, response, nil
}

func chooseFormat(formats []playerFormat, quality model.Quality) (playerFormat, bool) {
	var audio []playerFormat
	for _, format := range formats {
		if strings.HasPrefix(format.MimeType, "audio/") || format.AudioChannels > 0 {
			audio = append(audio, format)
		}
	}
	if len(audio) == 0 {
		return playerFormat{}, false
	}
	if quality == model.QualityLow {
		capped := audio[:0]
		for _, format := range audio {
			if format.Bitrate <= 128000 {
				capped = append(capped, format)
			}
		}
		if len(capped) != 0 {
			audio = capped
		}
	}
	sort.SliceStable(audio, func(i, j int) bool {
		if quality != model.QualityLow {
			ri, rj := audioQualityRank(audio[i].AudioQuality), audioQualityRank(audio[j].AudioQuality)
			if ri != rj {
				return ri > rj
			}
			if audio[i].AudioChannels != audio[j].AudioChannels {
				return audio[i].AudioChannels > audio[j].AudioChannels
			}
			ci, cj := codecScore(audio[i].MimeType), codecScore(audio[j].MimeType)
			if ci != cj {
				return ci > cj
			}
		}
		return audio[i].Bitrate > audio[j].Bitrate
	})
	return audio[0], true
}

func audioQualityRank(value string) int {
	switch {
	case strings.Contains(value, "HIGH"):
		return 3
	case strings.Contains(value, "MEDIUM"):
		return 2
	case strings.Contains(value, "LOW"):
		return 1
	default:
		return 0
	}
}

func codecScore(mime string) int {
	switch {
	case strings.Contains(mime, "opus"):
		return 3
	case strings.Contains(mime, "mp4a"):
		return 2
	default:
		return 1
	}
}

func codecFromMIME(mime string) string {
	if start := strings.Index(mime, `codecs="`); start >= 0 {
		value := mime[start+8:]
		if end := strings.IndexByte(value, '"'); end >= 0 {
			return value[:end]
		}
	}
	if media, _, ok := strings.Cut(mime, ";"); ok {
		return strings.TrimPrefix(media, "audio/")
	}
	return strings.TrimPrefix(mime, "audio/")
}

func expiryFromURL(raw string) *int64 {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	value, err := strconv.ParseInt(parsed.Query().Get("expire"), 10, 64)
	if err != nil || value == 0 {
		return nil
	}
	return &value
}

func (c *Client) resolveYTDLP(ctx context.Context, videoID string, quality model.Quality) (model.Stream, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args := []string{"-J", "--no-playlist", "-f", "bestaudio"}
	c.mu.RLock()
	proxy := c.proxy
	c.mu.RUnlock()
	if proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	args = append(args, "https://music.youtube.com/watch?v="+videoID)
	out, err := exec.CommandContext(ctx, "yt-dlp", args...).Output()
	if err != nil {
		return model.Stream{}, fmt.Errorf("yt-dlp: %w", err)
	}
	var root struct {
		URL         string            `json:"url"`
		FormatID    string            `json:"format_id"`
		Ext         string            `json:"ext"`
		ACodec      string            `json:"acodec"`
		ABR         float64           `json:"abr"`
		Loudness    *float64          `json:"loudness"`
		HTTPHeaders map[string]string `json:"http_headers"`
		Formats     []struct {
			URL, FormatID, ACodec, VCodec, Ext string
			ABR, TBR                           float64
		} `json:"formats"`
	}
	if err := json.Unmarshal(out, &root); err != nil {
		return model.Stream{}, fmt.Errorf("yt-dlp: %w", err)
	}
	stream := model.Stream{URL: root.URL, Headers: root.HTTPHeaders, UserAgent: root.HTTPHeaders["User-Agent"], Codec: root.ACodec, Bitrate: int(root.ABR * 1000), LoudnessDB: root.Loudness}
	if stream.URL == "" || quality != model.QualityHigh {
		type candidate struct {
			url, codec string
			bitrate    int
		}
		var options []candidate
		for _, format := range root.Formats {
			if format.URL == "" || format.ACodec == "none" || (format.VCodec != "" && format.VCodec != "none") {
				continue
			}
			bitrate := int(format.ABR * 1000)
			if bitrate == 0 {
				bitrate = int(format.TBR * 1000)
			}
			options = append(options, candidate{format.URL, format.ACodec, bitrate})
		}
		if len(options) == 0 && root.URL != "" {
			options = append(options, candidate{root.URL, root.ACodec, int(root.ABR * 1000)})
		}
		if len(options) == 0 {
			return model.Stream{}, errors.New("yt-dlp: no audio stream")
		}
		if quality == model.QualityHigh {
			sort.SliceStable(options, func(i, j int) bool {
				ci, cj := codecScore(options[i].codec), codecScore(options[j].codec)
				if ci != cj {
					return ci > cj
				}
				return options[i].bitrate > options[j].bitrate
			})
		} else {
			capped := options[:0]
			for _, option := range options {
				if option.bitrate <= 128000 {
					capped = append(capped, option)
				}
			}
			if len(capped) != 0 {
				options = capped
				sort.SliceStable(options, func(i, j int) bool { return options[i].bitrate > options[j].bitrate })
			} else {
				sort.SliceStable(options, func(i, j int) bool { return options[i].bitrate < options[j].bitrate })
			}
		}
		stream.URL, stream.Codec, stream.Bitrate = options[0].url, options[0].codec, options[0].bitrate
	}
	stream.ExpiresAt = expiryFromURL(stream.URL)
	return stream, nil
}

func (c *Client) trackPlayback(response playerResponse, playlistID string) {
	base := response.PlaybackTracking.VideostatsPlaybackURL.BaseURL
	if base == "" {
		return
	}
	go func() {
		parsed, err := url.Parse(base)
		if err != nil {
			return
		}
		query := parsed.Query()
		query.Set("c", "WEB_REMIX")
		query.Set("cpn", playbackNonce())
		query.Set("ver", "2")
		if playlistID != "" {
			query.Set("list", playlistID)
			query.Set("referrer", playlistID)
		}
		parsed.RawQuery = query.Encode()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
		if err != nil {
			return
		}
		resp, err := c.http.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
}

func playbackNonce() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "0000000000000000"
	}
	return base64.RawURLEncoding.EncodeToString(value[:])
}
