package downloads

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const interruptedMessage = "Download interrupted when Ryotunes stopped. Retry to download it again."

// Searcher resolves a labelled Spotify search to a YouTube id or HTTPS link.
type Searcher interface {
	// Match returns the selected YouTube id or canonical watch URL.
	Match(ctx context.Context, query string) (string, error)
}

// SoundCloudResolver resolves an internal SoundCloud track id to its public permalink.
type SoundCloudResolver interface {
	// Permalink returns the public HTTPS SoundCloud track URL.
	Permalink(ctx context.Context, trackID string) (string, error)
}

// LyricsSource resolves lyrics through the daemon's normal provider/cache path.
type LyricsSource interface {
	// Lyrics resolves cached or provider lyrics for one track.
	Lyrics(ctx context.Context, params protocol.LyricsParams) (*model.Lyrics, error)
}

// Options configures a download Manager.
type Options struct {
	// DataDir contains downloads.json.
	DataDir string
	// Sink receives downloads-changed snapshots.
	Sink bus.Sink
	// Searcher optionally resolves Spotify-labelled YouTube matches.
	Searcher Searcher
	// SoundCloud resolves internal SoundCloud ids.
	SoundCloud SoundCloudResolver
	// Lyrics resolves companion and embedded lyrics.
	Lyrics LyricsSource
	// HTTPClient fetches trusted artwork and iTunes metadata.
	HTTPClient *http.Client
	// Log receives asynchronous worker diagnostics.
	Log *slog.Logger
	// YTDLP overrides the yt-dlp executable path.
	YTDLP string
	// FFmpeg overrides the ffmpeg executable path.
	FFmpeg string
}

type attempt struct {
	cancel chan struct{}
	once   sync.Once
}

func (a *attempt) stop() { a.once.Do(func() { close(a.cancel) }) }

type downloadFileIndex map[string][]string

// Manager owns download state, worker admission, and publication.
type Manager struct {
	mu         sync.Mutex
	files      atomic.Pointer[downloadFileIndex]
	dataDir    string
	settings   model.DownloadSettings
	jobs       []model.DownloadEntry
	running    map[string]*attempt
	closed     bool
	progressAt map[string]time.Time
	sink       bus.Sink
	searcher   Searcher
	soundCloud SoundCloudResolver
	lyrics     LyricsSource
	http       *http.Client
	log        *slog.Logger
	ytdlp      string
	ffmpeg     string
	wg         sync.WaitGroup
}

// New loads durable state, recovers interrupted jobs, and starts queued work.
func New(options Options) (*Manager, error) {
	if options.DataDir == "" {
		return nil, errors.New("downloads data directory is required")
	}
	state, err := loadState(options.DataDir)
	if err != nil {
		return nil, err
	}
	if options.Sink == nil {
		options.Sink = bus.Discard
	}
	if options.Log == nil {
		options.Log = slog.Default()
	}
	if options.HTTPClient == nil {
		options.HTTPClient = netx.Client(0)
	}
	if options.YTDLP == "" {
		options.YTDLP = "yt-dlp"
	}
	if options.FFmpeg == "" {
		options.FFmpeg = "ffmpeg"
	}
	manager := &Manager{
		dataDir: options.DataDir, settings: state.Settings, jobs: state.Jobs,
		running: make(map[string]*attempt), progressAt: make(map[string]time.Time), sink: options.Sink, searcher: options.Searcher,
		soundCloud: options.SoundCloud, lyrics: options.Lyrics, http: options.HTTPClient,
		log: options.Log, ytdlp: options.YTDLP, ffmpeg: options.FFmpeg,
	}
	recovered := false
	now := time.Now().Unix()
	for index := range manager.jobs {
		job := &manager.jobs[index]
		if job.Status == model.DownloadStatusQueued || job.Status == model.DownloadStatusDownloading {
			message := interruptedMessage
			job.Status = model.DownloadStatusFailed
			job.Error = &message
			job.FinishedAt = &now
			recovered = true
		}
		if job.Warnings == nil {
			job.Warnings = []string{}
		}
	}
	manager.pruneLocked()
	if recovered {
		if err := manager.saveLocked(); err != nil {
			return nil, err
		}
	} else {
		manager.rebuildFileIndexLocked()
	}
	manager.pump()
	return manager, nil
}

// Settings returns the current sanitized download policy.
func (m *Manager) Settings() model.DownloadSettings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings
}

// SetSettings validates, persists, and applies policy to future attempts.
func (m *Manager) SetSettings(settings model.DownloadSettings) (model.DownloadSettings, error) {
	sanitized, err := validateSettings(settings)
	if err != nil {
		return settings, err
	}
	m.mu.Lock()
	previous := m.settings
	m.settings = sanitized
	if err := m.saveLocked(); err != nil {
		m.settings = previous
		m.mu.Unlock()
		return previous, fmt.Errorf("Could not save download preferences: %v", err)
	}
	m.mu.Unlock()
	m.pump()
	return sanitized, nil
}

// Snapshot returns jobs newest-first with current running and queued counts.
func (m *Manager) Snapshot() model.DownloadSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

func (m *Manager) snapshotLocked() model.DownloadSnapshot {
	jobs := make([]model.DownloadEntry, len(m.jobs))
	for index := range m.jobs {
		jobs[len(m.jobs)-1-index] = m.jobs[index]
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].CreatedAt > jobs[j].CreatedAt })
	queued := 0
	for _, job := range m.jobs {
		if job.Status == model.DownloadStatusQueued {
			queued++
		}
	}
	return model.DownloadSnapshot{Jobs: jobs, ActiveCount: len(m.running), QueuedCount: queued}
}

func (m *Manager) emit(snapshot model.DownloadSnapshot) {
	m.sink.Emit(protocol.EventDownloadsChanged, snapshot)
}

// Active reports whether queued or reserved download work inhibits idle exit.
func (m *Manager) Active() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.running) > 0 {
		return true
	}
	for _, job := range m.jobs {
		if job.Status == model.DownloadStatusQueued {
			return true
		}
	}
	return false
}

// Enqueue admits one supported track and captures current settings.
func (m *Manager) Enqueue(params protocol.EnqueueDownloadParams) (model.DownloadEntry, error) {
	artists, thumbnail := "", ""
	if params.Artists != nil {
		artists = *params.Artists
	}
	if params.Thumbnail != nil {
		thumbnail = *params.Thumbnail
	}
	entry := model.DownloadCollectionEntry{VideoID: params.VideoID, Title: params.Title, Artists: artists, Thumbnail: thumbnail}
	if err := validateEntry(entry); err != nil {
		return model.DownloadEntry{}, err
	}
	entry.Title = strings.TrimSpace(entry.Title)
	entry.Artists = strings.TrimSpace(entry.Artists)
	entry.Thumbnail = strings.TrimSpace(entry.Thumbnail)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return model.DownloadEntry{}, errStopping
	}
	if existing, ok := m.existingJobLocked(entry.VideoID); ok {
		m.mu.Unlock()
		return existing, nil
	}
	if path, ok := buildFolderIndex(m.settings.Path).find(m.settings, entry); ok {
		existing := m.existingFileEntryLocked(entry, path)
		m.mu.Unlock()
		return existing, nil
	}
	if m.admissionLocked() >= maxActive {
		m.mu.Unlock()
		return model.DownloadEntry{}, ErrQueueFull
	}
	job := m.makeJobLocked(entry)
	m.jobs = append(m.jobs, job)
	if err := m.saveLocked(); err != nil {
		m.jobs = m.jobs[:len(m.jobs)-1]
		m.mu.Unlock()
		return model.DownloadEntry{}, fmt.Errorf("Could not save the download queue: %v", err)
	}
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	m.emit(snapshot)
	m.pump()
	return job, nil
}

// EnqueueCollection admits up to 200 collection entries as one atomic state change.
func (m *Manager) EnqueueCollection(entries []model.DownloadCollectionEntry) (model.DownloadCollectionResult, error) {
	if len(entries) == 0 {
		return model.DownloadCollectionResult{}, errors.New("This collection has no tracks to download.")
	}
	if len(entries) > maxActive {
		return model.DownloadCollectionResult{}, ErrCollectionLarge
	}
	result := model.DownloadCollectionResult{AlreadyPaths: []string{}}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return model.DownloadCollectionResult{}, errStopping
	}
	available := max(0, maxActive-m.admissionLocked())
	pending := make([]model.DownloadEntry, 0, min(len(entries), available))
	seen := make(map[string]bool)
	index := buildFolderIndex(m.settings.Path)
	for _, entry := range entries {
		if err := validateEntry(entry); err != nil {
			result.Skipped++
			continue
		}
		entry.Title = strings.TrimSpace(entry.Title)
		entry.Artists = strings.TrimSpace(entry.Artists)
		entry.Thumbnail = strings.TrimSpace(entry.Thumbnail)
		if seen[entry.VideoID] {
			result.AlreadyDownloaded++
			continue
		}
		seen[entry.VideoID] = true
		if _, ok := m.existingJobLocked(entry.VideoID); ok {
			result.AlreadyDownloaded++
			continue
		}
		if path, ok := index.find(m.settings, entry); ok {
			result.AlreadyDownloaded++
			if len(result.AlreadyPaths) < 6 {
				result.AlreadyPaths = append(result.AlreadyPaths, path)
			}
			continue
		}
		if len(pending) >= available {
			result.Skipped++
			continue
		}
		pending = append(pending, m.makeJobLocked(entry))
	}
	if len(pending) == 0 {
		m.mu.Unlock()
		return result, nil
	}
	oldLen := len(m.jobs)
	m.jobs = append(m.jobs, pending...)
	result.Added = len(pending)
	if err := m.saveLocked(); err != nil {
		m.jobs = m.jobs[:oldLen]
		m.mu.Unlock()
		return model.DownloadCollectionResult{}, fmt.Errorf("Could not save the download queue: %v", err)
	}
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	m.emit(snapshot)
	m.pump()
	return result, nil
}

func (m *Manager) makeJobLocked(entry model.DownloadCollectionEntry) model.DownloadEntry {
	collection := trimOptional(entry.Collection, 120)
	kind := trimOptional(entry.CollectionKind, 20)
	if kind != nil && *kind != "album" && *kind != "playlist" {
		kind = nil
	}
	source, _ := sourceOf(entry.VideoID)
	return model.DownloadEntry{
		ID: newJobID(), VideoID: entry.VideoID, Title: entry.Title, Artists: entry.Artists,
		Thumbnail: entry.Thumbnail, Status: model.DownloadStatusQueued, Source: source,
		Collection: collection, CollectionKind: kind, CreatedAt: time.Now().Unix(),
		Settings: m.settings, Warnings: []string{},
	}
}

func trimOptional(value *string, limit int) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	for len(trimmed) > limit {
		_, size := utf8.DecodeLastRuneInString(trimmed)
		trimmed = trimmed[:len(trimmed)-size]
	}
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func sourceOf(id string) (string, error) {
	switch {
	case youtubeIDPattern.MatchString(id):
		return "YouTube", nil
	case soundCloudPattern.MatchString(id):
		return "SoundCloud", nil
	case spotifyPattern.MatchString(id):
		return "YouTube match for Spotify", nil
	case strings.HasPrefix(id, "LOCAL"):
		return "", errors.New("This track is already a local file on your computer.")
	case strings.HasPrefix(id, "RYOTUNES_RADIO:"):
		return "", errors.New("Radio stations are live streams and can't be downloaded.")
	default:
		return "", errors.New("This track can't be downloaded.")
	}
}

func (m *Manager) admissionLocked() int {
	queued := 0
	for _, job := range m.jobs {
		if job.Status == model.DownloadStatusQueued {
			queued++
		}
	}
	return queued + len(m.running)
}

func validateEntry(entry model.DownloadCollectionEntry) error {
	if _, err := sourceOf(entry.VideoID); err != nil {
		return err
	}
	if len(entry.VideoID) > 80 || len(entry.Title) > 512 || len(entry.Artists) > 512 || len(entry.Thumbnail) > 4096 {
		return errors.New("The track metadata is too large to download safely.")
	}
	if strings.TrimSpace(entry.Title) == "" {
		return errors.New("This track has no title to use for its download.")
	}
	return nil
}

func (m *Manager) existingJobLocked(videoID string) (model.DownloadEntry, bool) {
	for index := len(m.jobs) - 1; index >= 0; index-- {
		job := m.jobs[index]
		if job.VideoID != videoID {
			continue
		}
		if job.Status == model.DownloadStatusQueued || job.Status == model.DownloadStatusDownloading {
			return job, true
		}
		if job.Status == model.DownloadStatusCompleted && job.FilePath != nil {
			if info, err := os.Stat(*job.FilePath); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
				return job, true
			}
		}
	}
	return model.DownloadEntry{}, false
}

func (m *Manager) existingFileEntryLocked(entry model.DownloadCollectionEntry, path string) model.DownloadEntry {
	source, _ := sourceOf(entry.VideoID)
	finished := time.Now().Unix()
	if info, err := os.Stat(path); err == nil {
		finished = info.ModTime().Unix()
	}
	return model.DownloadEntry{
		ID: newJobID(), VideoID: entry.VideoID, Title: entry.Title, Artists: entry.Artists,
		Thumbnail: entry.Thumbnail, Status: model.DownloadStatusCompleted, Progress: 100,
		FilePath: &path, Source: source, CreatedAt: finished, FinishedAt: &finished,
		Settings: m.settings, Warnings: []string{},
	}
}

// Cancel marks an active attempt cancelled and terminates its process group.
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	index := m.jobIndexLocked(id)
	if index < 0 {
		m.mu.Unlock()
		return ErrMissingJob
	}
	job := &m.jobs[index]
	if job.Status != model.DownloadStatusQueued && job.Status != model.DownloadStatusDownloading {
		m.mu.Unlock()
		return nil
	}
	previous := append([]model.DownloadEntry(nil), m.jobs...)
	now := time.Now().Unix()
	job.Status = model.DownloadStatusCancelled
	job.Progress = min(job.Progress, 99)
	job.FinishedAt = &now
	job.Error = nil
	m.pruneLocked()
	if err := m.saveLocked(); err != nil {
		m.jobs = previous
		m.mu.Unlock()
		return fmt.Errorf("Download cancelled, but history could not be saved: %v", err)
	}
	if running := m.running[id]; running != nil {
		running.stop()
	}
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	m.emit(snapshot)
	m.pump()
	return nil
}

// Retry replaces a failed or cancelled row with a fresh attempt identity.
func (m *Manager) Retry(id string) (model.DownloadEntry, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return model.DownloadEntry{}, errStopping
	}
	if m.admissionLocked() >= maxActive {
		m.mu.Unlock()
		return model.DownloadEntry{}, ErrQueueFull
	}
	index := m.jobIndexLocked(id)
	if index < 0 {
		m.mu.Unlock()
		return model.DownloadEntry{}, ErrMissingJob
	}
	old := m.jobs[index]
	if old.Status != model.DownloadStatusFailed && old.Status != model.DownloadStatusCancelled {
		m.mu.Unlock()
		return model.DownloadEntry{}, ErrRetryState
	}
	fresh := old
	fresh.ID = newJobID()
	fresh.Status = model.DownloadStatusQueued
	fresh.Progress = 0
	fresh.FilePath = nil
	fresh.Error = nil
	fresh.CreatedAt = time.Now().Unix()
	fresh.FinishedAt = nil
	fresh.Settings = m.settings
	fresh.Warnings = []string{}
	previous := m.jobs
	next := make([]model.DownloadEntry, 0, len(previous))
	next = append(next, previous[:index]...)
	next = append(next, previous[index+1:]...)
	m.jobs = append(next, fresh)
	if err := m.saveLocked(); err != nil {
		m.jobs = previous
		m.mu.Unlock()
		return model.DownloadEntry{}, fmt.Errorf("Could not save the download queue: %v", err)
	}
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	m.emit(snapshot)
	m.pump()
	return fresh, nil
}

// ClearHistory removes terminal rows without touching downloaded files.
func (m *Manager) ClearHistory() error {
	m.mu.Lock()
	old := m.jobs
	kept := make([]model.DownloadEntry, 0, len(old))
	for _, job := range old {
		if job.Status == model.DownloadStatusQueued || job.Status == model.DownloadStatusDownloading {
			kept = append(kept, job)
		}
	}
	m.jobs = kept
	if err := m.saveLocked(); err != nil {
		m.jobs = old
		m.mu.Unlock()
		return fmt.Errorf("Could not clear download history: %v", err)
	}
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	m.emit(snapshot)
	return nil
}

// Stream returns the newest playable downloaded file for a provider track.
func (m *Manager) Stream(videoID string) (model.Stream, bool) {
	index := m.files.Load()
	if index == nil {
		return model.Stream{}, false
	}
	for _, path := range (*index)[videoID] {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			continue
		}
		return model.Stream{URL: (&url.URL{Scheme: "file", Path: path}).String()}, true
	}
	return model.Stream{}, false
}

// Open opens the parent folder of a completed download.
func (m *Manager) Open(id string) error {
	m.mu.Lock()
	index := m.jobIndexLocked(id)
	if index < 0 {
		m.mu.Unlock()
		return errors.New("That download has not finished yet.")
	}
	job := m.jobs[index]
	m.mu.Unlock()
	if job.Status != model.DownloadStatusCompleted || job.FilePath == nil {
		return errors.New("That download has not finished yet.")
	}
	info, err := os.Stat(*job.FilePath)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("The downloaded file has moved or been deleted.")
	}
	parent := filepath.Dir(*job.FilePath)
	if parent == "." || parent == "" {
		return errors.New("The download has no parent folder.")
	}
	return openFolder(parent)
}

// OpenFolder creates and opens the configured destination.
func (m *Manager) OpenFolder() error {
	m.mu.Lock()
	path := m.settings.Path
	m.mu.Unlock()
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("Could not create the download folder: %v", err)
	}
	return openFolder(path)
}

func openFolder(path string) error {
	command := exec.Command("xdg-open", path)
	command.Stdin = nil
	command.Stdout = nil
	command.Stderr = nil
	if err := command.Start(); err != nil {
		return fmt.Errorf("Could not open the folder: %v", err)
	}
	go func() { _ = command.Wait() }()
	return nil
}

// Shutdown marks unfinished work retryable, terminates process groups, and awaits teardown.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	var saveErr error
	if !m.closed {
		m.closed = true
		now := time.Now().Unix()
		for index := range m.jobs {
			job := &m.jobs[index]
			if job.Status == model.DownloadStatusQueued || job.Status == model.DownloadStatusDownloading {
				message := interruptedMessage
				job.Status = model.DownloadStatusFailed
				job.Error = &message
				job.FinishedAt = &now
			}
		}
		for _, running := range m.running {
			running.stop()
		}
		m.pruneLocked()
		saveErr = m.saveLocked()
	}
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
		return saveErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) jobIndexLocked(id string) int {
	for index := range m.jobs {
		if m.jobs[index].ID == id {
			return index
		}
	}
	return -1
}

func (m *Manager) saveLocked() error {
	if err := saveState(m.dataDir, diskState{Settings: m.settings, Jobs: m.jobs}); err != nil {
		return err
	}
	m.rebuildFileIndexLocked()
	return nil
}

func (m *Manager) rebuildFileIndexLocked() {
	index := make(downloadFileIndex)
	for position := len(m.jobs) - 1; position >= 0; position-- {
		job := m.jobs[position]
		if job.Status == model.DownloadStatusCompleted && job.FilePath != nil {
			index[job.VideoID] = append(index[job.VideoID], *job.FilePath)
		}
	}
	m.files.Store(&index)
}

func (m *Manager) pruneLocked() {
	terminal := make([]int, 0, len(m.jobs))
	for index, job := range m.jobs {
		if job.Status != model.DownloadStatusQueued && job.Status != model.DownloadStatusDownloading {
			terminal = append(terminal, index)
		}
	}
	if len(terminal) <= 200 {
		return
	}
	remove := make(map[int]bool, len(terminal)-200)
	for _, index := range terminal[:len(terminal)-200] {
		remove[index] = true
	}
	kept := m.jobs[:0]
	for index, job := range m.jobs {
		if !remove[index] {
			kept = append(kept, job)
		}
	}
	m.jobs = kept
}

func validateSoundCloudURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("SoundCloud returned an invalid track link.")
	}
	host := strings.ToLower(parsed.Hostname())
	if parsed.Scheme != "https" || (host != "soundcloud.com" && host != "www.soundcloud.com") {
		return errors.New("SoundCloud returned an unsupported track link.")
	}
	return nil
}
