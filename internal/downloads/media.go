package downloads

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type artwork struct {
	bytes     []byte
	extension string
}

type artworkResult struct {
	art     *artwork
	warning string
}

type lyricsResult struct {
	lyrics  *model.Lyrics
	warning string
}

func (m *Manager) enrich(parent context.Context, audio string, job model.DownloadEntry) (sidecars []string, warnings []string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			sidecars = nil
			warnings = []string{truncateError(fmt.Sprintf("Metadata enrichment did not finish: %v", recovered))}
		}
	}()
	duration := probeDuration(parent, audio)
	var artResult artworkResult
	var lyricResult lyricsResult
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				artResult.warning = truncateError(fmt.Sprintf("Metadata enrichment did not finish: %v", recovered))
			}
			wait.Done()
		}()
		artResult = m.resolveArtwork(parent, job, duration)
	}()
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				lyricResult.warning = truncateError(fmt.Sprintf("Metadata enrichment did not finish: %v", recovered))
			}
			wait.Done()
		}()
		lyricResult = m.resolveLyrics(parent, job, duration)
	}()
	wait.Wait()
	warnings = []string{}
	if artResult.warning != "" {
		warnings = append(warnings, artResult.warning)
	}
	if lyricResult.warning != "" {
		warnings = append(warnings, lyricResult.warning)
	}
	sidecars = []string{}
	var artPath string
	if artResult.art != nil {
		artPath = filepath.Join(filepath.Dir(audio), "audio."+artResult.art.extension)
		if err := os.WriteFile(artPath, artResult.art.bytes, 0o600); err != nil {
			warnings = append(warnings, truncateError(fmt.Sprintf("Could not save the cover art file: %v", err)))
			artPath = ""
		} else {
			sidecars = append(sidecars, artPath)
		}
	}
	plainLyrics := ""
	if lyricResult.lyrics != nil {
		plainLyrics = plainLyricText(*lyricResult.lyrics)
		if strings.TrimSpace(plainLyrics) == "" {
			warnings = append(warnings, "No lyrics were found for this track.")
		} else {
			extension, content := "txt", plainLyrics
			if lyricResult.lyrics.Synced {
				if synced := lrcText(*lyricResult.lyrics); synced != "" {
					extension, content = "lrc", synced
				}
			}
			path := filepath.Join(filepath.Dir(audio), "audio."+extension)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				warnings = append(warnings, truncateError(fmt.Sprintf("Could not save the lyrics file: %v", err)))
			} else {
				sidecars = append(sidecars, path)
			}
		}
	}
	album := ""
	if job.CollectionKind != nil && *job.CollectionKind == "album" && job.Collection != nil {
		album = *job.Collection
	}
	if err := m.embedMetadata(parent, audio, artPath, job.Title, job.Artists, album, plainLyrics); err != nil {
		warnings = append(warnings, truncateError(fmt.Sprintf("Could not embed metadata into the audio file: %v", err)))
	}
	return sidecars, warnings
}

func (m *Manager) resolveArtwork(parent context.Context, job model.DownloadEntry, duration *float64) artworkResult {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if trustedArtworkURL(job.Thumbnail, []string{".ytimg.com", ".ggpht.com", ".googleusercontent.com", ".sndcdn.com", ".scdn.co"}) {
		if art, err := m.fetchArtwork(ctx, job.Thumbnail); err == nil {
			return artworkResult{art: art}
		}
	}
	artURL, err := m.itunesArtwork(ctx, job, duration)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return artworkResult{warning: "Cover art lookup timed out."}
	}
	if err != nil {
		return artworkResult{warning: "Cover art could not be downloaded."}
	}
	if artURL == "" {
		return artworkResult{warning: "No matching cover art was found for this track."}
	}
	art, err := m.fetchArtwork(ctx, artURL)
	if err != nil {
		return artworkResult{warning: "Cover art could not be downloaded."}
	}
	return artworkResult{art: art}
}

func trustedArtworkURL(raw string, suffixes []string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || net.ParseIP(host) != nil || host == "localhost" {
		return false
	}
	for _, suffix := range suffixes {
		base := strings.TrimPrefix(suffix, ".")
		if host == base || strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func (m *Manager) fetchArtwork(ctx context.Context, raw string) (*artwork, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	response, err := m.doTrusted(request, []string{".ytimg.com", ".ggpht.com", ".googleusercontent.com", ".sndcdn.com", ".scdn.co", ".mzstatic.com"})
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 12<<20+1))
	if err != nil || len(data) == 0 || len(data) > 12<<20 {
		return nil, errors.New("invalid cover art body")
	}
	extension := ""
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		extension = "jpg"
	} else if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		extension = "png"
	}
	if extension == "" {
		return nil, errors.New("cover art is not JPEG or PNG")
	}
	return &artwork{bytes: data, extension: extension}, nil
}

type itunesResponse struct {
	// Results contains matching catalogue songs.
	Results []struct {
		// TrackName is the catalogue song title.
		TrackName string `json:"trackName"`
		// ArtistName is the catalogue artist credit.
		ArtistName string `json:"artistName"`
		// ArtworkURL100 is the small artwork template URL.
		ArtworkURL100 string `json:"artworkUrl100"`
		// TrackTimeMillis is the catalogue duration.
		TrackTimeMillis float64 `json:"trackTimeMillis"`
	} `json:"results"`
}

func (m *Manager) itunesArtwork(parent context.Context, job model.DownloadEntry, duration *float64) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	endpoint, _ := url.Parse("https://itunes.apple.com/search")
	query := endpoint.Query()
	query.Set("media", "music")
	query.Set("entity", "song")
	query.Set("limit", "25")
	query.Set("country", "US")
	query.Set("term", strings.TrimSpace(job.Artists+" "+job.Title))
	endpoint.RawQuery = query.Encode()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	response, err := m.doTrusted(request, []string{".apple.com"})
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return "", errors.New("invalid iTunes response")
	}
	var payload itunesResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", err
	}
	title := normalizeMatch(job.Title)
	artist := normalizeMatch(job.Artists)
	primary := normalizeMatch(primaryArtist(job.Artists))
	for _, result := range payload.Results {
		if normalizeMatch(result.TrackName) != title {
			continue
		}
		candidateArtist := normalizeMatch(result.ArtistName)
		if candidateArtist != artist && candidateArtist != primary {
			continue
		}
		if duration != nil {
			if result.TrackTimeMillis <= 0 || abs(result.TrackTimeMillis/1000-*duration) > 5 {
				continue
			}
		}
		artURL := strings.Replace(result.ArtworkURL100, "100x100bb", "600x600bb", 1)
		if trustedArtworkURL(artURL, []string{".mzstatic.com"}) {
			return artURL, nil
		}
	}
	return "", nil
}
func (m *Manager) doTrusted(request *http.Request, suffixes []string) (*http.Response, error) {
	client := *m.http
	client.CheckRedirect = func(next *http.Request, _ []*http.Request) error {
		if !trustedArtworkURL(next.URL.String(), suffixes) {
			return errors.New("untrusted redirect")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if !trustedArtworkURL(response.Request.URL.String(), suffixes) {
		_ = response.Body.Close()
		return nil, errors.New("untrusted response URL")
	}
	return response, nil
}

func normalizeMatch(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func (m *Manager) resolveLyrics(parent context.Context, job model.DownloadEntry, duration *float64) lyricsResult {
	if m.lyrics == nil {
		return lyricsResult{warning: "No lyrics were found for this track."}
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	lyrics, err := m.lyrics.Lyrics(ctx, protocol.LyricsParams{VideoID: job.VideoID, Title: job.Title, Artists: job.Artists, Duration: duration})
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return lyricsResult{warning: "Lyrics lookup timed out."}
	}
	if err != nil || lyrics == nil {
		return lyricsResult{warning: "No lyrics were found for this track."}
	}
	if lyrics.Instrumental {
		return lyricsResult{}
	}
	return lyricsResult{lyrics: lyrics}
}

func plainLyricText(lyrics model.Lyrics) string {
	lines := make([]string, 0, len(lyrics.Lines))
	for _, line := range lyrics.Lines {
		lines = append(lines, line.Text)
	}
	return strings.Join(lines, "\n")
}

func lrcText(lyrics model.Lyrics) string {
	var b strings.Builder
	for _, line := range lyrics.Lines {
		if line.TimeMS == nil {
			continue
		}
		milliseconds := *line.TimeMS
		minutes := milliseconds / 60000
		seconds := (milliseconds % 60000) / 1000
		centiseconds := (milliseconds % 1000) / 10
		fmt.Fprintf(&b, "[%02d:%02d.%02d]%s\n", minutes, seconds, centiseconds, line.Text)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func probeDuration(parent context.Context, audio string) *float64 {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", audio).Output()
	if err != nil {
		return nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || value <= 0 {
		return nil
	}
	return &value
}

func (m *Manager) embedMetadata(parent context.Context, audio, artPath, title, artist, album, lyrics string) error {
	extension := strings.ToLower(strings.TrimPrefix(filepath.Ext(audio), "."))
	if extension != "mp3" && extension != "flac" && extension != "m4a" && extension != "opus" && extension != "ogg" {
		return nil
	}
	temporary := strings.TrimSuffix(audio, filepath.Ext(audio)) + ".rytag." + extension
	_ = os.Remove(temporary)
	defer os.Remove(temporary)
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	args := []string{"-v", "error", "-nostdin", "-i", audio}
	if extension == "opus" || extension == "ogg" {
		args = append(args, "-map", "0:a:0", "-c:a", "copy", "-map_metadata", "-1",
			"-metadata", "title="+title, "-metadata", "artist="+artist, "-metadata", "album="+album)
		if lyrics != "" {
			args = append(args, "-metadata", "LYRICS="+lyrics)
		}
		if artPath != "" {
			picture, err := metadataBlockPicture(artPath)
			if err != nil {
				return err
			}
			args = append(args, "-metadata", "METADATA_BLOCK_PICTURE="+picture)
		}
	} else {
		if artPath != "" {
			args = append(args, "-i", artPath, "-map", "0:a:0", "-map", "1:v:0", "-c", "copy", "-disposition:v:0", "attached_pic", "-metadata:s:v:0", "title=Album cover", "-metadata:s:v:0", "comment=Cover (front)")
		} else {
			args = append(args, "-map", "0:a:0", "-c", "copy")
		}
		args = append(args, "-metadata", "title="+title, "-metadata", "artist="+artist)
		if album != "" {
			args = append(args, "-metadata", "album="+album)
		}
		if lyrics != "" {
			args = append(args, "-metadata", "lyrics="+lyrics)
		}
		if extension == "mp3" {
			args = append(args, "-id3v2_version", "3")
		}
	}
	args = append(args, temporary)
	output, err := exec.CommandContext(ctx, m.ffmpeg, args...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return errors.New(message)
		}
		return err
	}
	file, err := os.Open(temporary)
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, audio)
}

func metadataBlockPicture(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return encodeMetadataBlockPicture(data)
}

func encodeMetadataBlockPicture(data []byte) (string, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	mime, depth := "", uint32(0)
	switch format {
	case "jpeg":
		mime, depth = "image/jpeg", 24
	case "png":
		mime = "image/png"
		if len(data) < 26 {
			return "", errors.New("PNG cover art has no IHDR")
		}
		channels := map[byte]uint32{0: 1, 2: 3, 3: 1, 4: 2, 6: 4}[data[25]]
		if channels == 0 {
			return "", errors.New("PNG cover art has an unsupported colour type")
		}
		depth = uint32(data[24]) * channels
	default:
		return "", errors.New("cover art is not JPEG or PNG")
	}
	block := make([]byte, 0, 32+len(mime)+len(data))
	block = binary.BigEndian.AppendUint32(block, 3)
	block = binary.BigEndian.AppendUint32(block, uint32(len(mime)))
	block = append(block, mime...)
	block = binary.BigEndian.AppendUint32(block, 0)
	block = binary.BigEndian.AppendUint32(block, uint32(config.Width))
	block = binary.BigEndian.AppendUint32(block, uint32(config.Height))
	block = binary.BigEndian.AppendUint32(block, depth)
	block = binary.BigEndian.AppendUint32(block, 0)
	block = binary.BigEndian.AppendUint32(block, uint32(len(data)))
	block = append(block, data...)
	return base64.StdEncoding.EncodeToString(block), nil
}
