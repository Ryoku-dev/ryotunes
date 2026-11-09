// Package downloads manages durable, bounded audio downloads and publication.
package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
)

const (
	stateFileName = "downloads.json"
	stagingName   = ".ryotunes-incomplete"
	maxJobs       = 400
	maxActive     = 200
	maxStateBytes = 8 << 20
)

var jobSequence atomic.Uint64

var (
	youtubeIDPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	soundCloudPattern  = regexp.MustCompile(`^sc:track:[0-9]+$`)
	spotifyPattern     = regexp.MustCompile(`^spotify:track:[A-Za-z0-9]+$`)
	progressPattern    = regexp.MustCompile(`(?:^|\s)([0-9]+(?:\.[0-9]+)?)%(?:\s|$)`)
	downloadExtensions = map[string]bool{"mp3": true, "m4a": true, "opus": true, "ogg": true, "aac": true, "flac": true, "wav": true, "webm": true, "mp4": true}
)

var (
	// ErrQueueFull reports the 200-track admission ceiling.
	ErrQueueFull = errors.New("The download queue is full (200 tracks). Let some finish first.")
	// ErrCollectionLarge reports a collection larger than one admission batch.
	ErrCollectionLarge = errors.New("A collection can add at most 200 tracks at once.")
	// ErrMissingJob reports a stale download identity.
	ErrMissingJob = errors.New("That download is no longer in the list.")
	// ErrRetryState reports a non-terminal retry request.
	ErrRetryState = errors.New("Only failed or cancelled downloads can be retried.")
)

var errStopping = errors.New("Ryotunes is shutting down.")

type diskState struct {
	// Settings is the captured download policy.
	Settings model.DownloadSettings `json:"settings"`
	// Jobs is the active and terminal history.
	Jobs []model.DownloadEntry `json:"jobs"`
}

func newJobID() string {
	return "dl-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.FormatUint(jobSequence.Add(1), 10)
}

func validFormat(f model.DownloadFormat) bool {
	return f == model.DownloadOriginal || f == model.DownloadMP3 || f == model.DownloadOpus || f == model.DownloadFLAC
}

func defaultSettings() model.DownloadSettings {
	return model.DownloadSettings{
		Path:             filepath.Join(musicDirectory(), "Ryotunes"),
		Format:           model.DownloadOriginal,
		Workers:          1,
		OrganizeByArtist: true,
		EmbedMetadata:    true,
	}
}

func musicDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		home = string(filepath.Separator)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "xdg-user-dir", "MUSIC").Output(); err == nil {
		value := strings.TrimSpace(string(output))
		if filepath.IsAbs(value) && filepath.Clean(value) != filepath.Clean(home) {
			return value
		}
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	if data, err := os.ReadFile(filepath.Join(configHome, "user-dirs.dirs")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "XDG_MUSIC_DIR=") {
				continue
			}
			value := strings.Trim(strings.TrimPrefix(line, "XDG_MUSIC_DIR="), "\"")
			value = strings.ReplaceAll(value, "${HOME}", home)
			value = strings.ReplaceAll(value, "$HOME", home)
			if filepath.IsAbs(value) {
				return value
			}
		}
	}
	return filepath.Join(home, "Music")
}

func normalizeLoadedSettings(s model.DownloadSettings) model.DownloadSettings {
	if !validFormat(s.Format) {
		s.Format = model.DownloadOriginal
	}
	if s.Workers < 1 {
		s.Workers = 1
	} else if s.Workers > 4 {
		s.Workers = 4
	}
	return s
}

func validateSettings(s model.DownloadSettings) (model.DownloadSettings, error) {
	if s.Workers < 1 || s.Workers > 4 {
		return s, errors.New("Choose between 1 and 4 download workers.")
	}
	if !validFormat(s.Format) {
		s.Format = model.DownloadOriginal
	}
	path := strings.TrimSpace(s.Path)
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return s, errors.New("The download folder path is invalid.")
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if path == "" || !filepath.IsAbs(path) {
		return s, errors.New("The download folder path is invalid.")
	}
	path = filepath.Clean(path)
	if path == string(filepath.Separator) {
		return s, errors.New("Choose an absolute download folder, not the filesystem root.")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return s, fmt.Errorf("Could not create the download folder: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return s, fmt.Errorf("Could not create the download folder: %v", err)
	}
	if !info.IsDir() {
		return s, errors.New("The download destination is not a folder.")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return s, errors.New("The download folder path is invalid.")
	}
	s.Path = canonical
	return s, nil
}

func loadState(dataDir string) (diskState, error) {
	defaults := diskState{Settings: defaultSettings(), Jobs: []model.DownloadEntry{}}
	_ = os.Remove(filepath.Join(dataDir, stateFileName+".tmp"))
	file, err := os.Open(filepath.Join(dataDir, stateFileName))
	if errors.Is(err, os.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	defer file.Close()
	limited := io.LimitReader(file, maxStateBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return defaults, err
	}
	if len(data) > maxStateBytes {
		return defaults, errors.New("downloads.json is larger than 8 MiB")
	}
	state := defaults
	if err := json.Unmarshal(data, &state); err != nil {
		return defaults, err
	}
	if len(state.Jobs) > maxJobs {
		return defaults, errors.New("downloads.json contains more than 400 jobs")
	}
	state.Settings = normalizeLoadedSettings(state.Settings)
	if !filepath.IsAbs(state.Settings.Path) {
		return defaults, errors.New("saved download folder is not absolute")
	}
	if state.Jobs == nil {
		state.Jobs = []model.DownloadEntry{}
	}
	for index := range state.Jobs {
		settings := &state.Jobs[index].Settings
		if !validFormat(settings.Format) {
			settings.Format = model.DownloadOriginal
		}
		if settings.Workers < 1 {
			settings.Workers = 1
		} else if settings.Workers > 4 {
			settings.Workers = 4
		}
	}
	return state, nil
}

func saveState(dataDir string, state diskState) (err error) {
	if err = os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(dataDir, stateFileName+".tmp")
	_ = os.Remove(tmp)
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(tmp)
	}()
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	if err = encoder.Encode(state); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp, filepath.Join(dataDir, stateFileName)); err != nil {
		return err
	}
	dir, err := os.Open(dataDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func sanitizeComponent(value string) string {
	var b strings.Builder
	space := false
	for _, r := range value {
		if r == 0 || strings.ContainsRune(`/\\:*?"<>|`, r) {
			r = '_'
		} else if unicode.IsControl(r) {
			continue
		}
		if unicode.IsSpace(r) {
			if space {
				continue
			}
			r = ' '
			space = true
		} else {
			space = false
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), " .")
	for len(out) > 120 {
		_, size := utf8.DecodeLastRuneInString(out)
		out = out[:len(out)-size]
	}
	out = strings.TrimRight(out, " .")
	if out == "" {
		return "download"
	}
	return out
}

func primaryArtist(artists string) string {
	lower := strings.ToLower(artists)
	cut := len(artists)
	for _, delimiter := range []string{",", "&", "•", ";", " feat", " ft."} {
		if index := strings.Index(lower, delimiter); index >= 0 && index < cut {
			cut = index
		}
	}
	artist := strings.TrimSpace(artists[:cut])
	if artist == "" {
		return "Unknown Artist"
	}
	return artist
}

func finalPath(job model.DownloadEntry, extension string) string {
	root := job.Settings.Path
	if job.Collection != nil && strings.TrimSpace(*job.Collection) != "" {
		root = filepath.Join(root, sanitizeComponent(*job.Collection))
	} else if job.Settings.OrganizeByArtist {
		root = filepath.Join(root, sanitizeComponent(primaryArtist(job.Artists)))
	}
	base := strings.TrimSpace(job.Title)
	if !job.Settings.OrganizeByArtist && strings.TrimSpace(job.Artists) != "" {
		base = strings.TrimSpace(job.Artists) + " - " + base
	}
	stem := sanitizeComponent(base) + " [" + sanitizeComponent(job.VideoID) + "]"
	return filepath.Join(root, stem+"."+extension)
}

func parseProgress(line string) (int, bool) {
	if !strings.Contains(line, "[download]") {
		return 0, false
	}
	match := progressPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil || value != value {
		return 0, false
	}
	progress := int(value + 0.5)
	if progress < 0 {
		progress = 0
	}
	if progress > 99 {
		progress = 99
	}
	return progress, true
}

func buildYTDLPArgs(stage string, format model.DownloadFormat, embed bool, target string) []string {
	args := []string{
		"--ignore-config", "--no-plugin-dirs", "--no-playlist", "--no-exec", "--no-continue",
		"--newline", "--no-color", "--progress", "--progress-delta", "0.4",
		"--socket-timeout", "20", "--retries", "2", "--fragment-retries", "2",
		"--concurrent-fragments", "1", "--postprocessor-args", "ffmpeg:-threads 1",
		"-f", "bestaudio/best", "-x", "-o", filepath.Join(stage, "audio.%(ext)s"),
	}
	if proxy := netx.Proxy(); proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	switch format {
	case model.DownloadMP3:
		args = append(args, "--audio-format", "mp3", "--audio-quality", "320K")
	case model.DownloadOpus:
		args = append(args, "--audio-format", "opus", "--audio-quality", "160K")
	case model.DownloadFLAC:
		args = append(args, "--audio-format", "flac", "--postprocessor-args", "ExtractAudio:-sample_fmt s16")
	default:
		args = append(args, "--audio-format", "best")
	}
	if embed {
		args = append(args, "--embed-metadata")
	}
	return append(args, "--", target)
}
