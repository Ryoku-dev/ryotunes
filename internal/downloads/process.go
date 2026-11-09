package downloads

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func (m *Manager) pump() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	limit := m.settings.Workers
	if limit < 1 {
		limit = 1
	} else if limit > 4 {
		limit = 4
	}
	type launch struct {
		job model.DownloadEntry
		try *attempt
	}
	launches := make([]launch, 0, limit)
	for len(m.running) < limit {
		index := -1
		for candidate := range m.jobs {
			if m.jobs[candidate].Status == model.DownloadStatusQueued {
				index = candidate
				break
			}
		}
		if index < 0 {
			break
		}
		job := &m.jobs[index]
		job.Status = model.DownloadStatusDownloading
		try := &attempt{cancel: make(chan struct{})}
		m.running[job.ID] = try
		launches = append(launches, launch{job: *job, try: try})
	}
	if len(launches) == 0 {
		m.mu.Unlock()
		return
	}
	if err := m.saveLocked(); err != nil {
		now := time.Now().Unix()
		for _, launch := range launches {
			delete(m.running, launch.job.ID)
			if index := m.jobIndexLocked(launch.job.ID); index >= 0 {
				message := err.Error()
				m.jobs[index].Status = model.DownloadStatusFailed
				m.jobs[index].Error = &message
				m.jobs[index].FinishedAt = &now
			}
		}
		m.pruneLocked()
		snapshot := m.snapshotLocked()
		m.mu.Unlock()
		m.log.Error("download worker transition persistence failed", "error", err)
		m.emit(snapshot)
		return
	}
	snapshot := m.snapshotLocked()
	m.wg.Add(len(launches))
	m.mu.Unlock()
	m.emit(snapshot)
	for _, item := range launches {
		go func() {
			defer m.wg.Done()
			m.run(item.job, item.try)
		}()
	}
}

func (m *Manager) run(job model.DownloadEntry, running *attempt) {
	base, stop := context.WithCancel(context.Background())
	defer stop()
	go func() {
		select {
		case <-running.cancel:
			stop()
		case <-base.Done():
		}
	}()
	resolveCtx, cancelResolve := context.WithTimeout(base, 30*time.Minute)
	target, err := m.resolveTarget(resolveCtx, job)
	resolveErr := resolveCtx.Err()
	cancelResolve()
	if err != nil {
		if errors.Is(resolveErr, context.DeadlineExceeded) {
			err = errors.New("The download source took too long to respond.")
		}
		m.finish(job.ID, err)
		return
	}
	stage, err := createStaging(job)
	if err != nil {
		m.finish(job.ID, err)
		return
	}
	defer os.RemoveAll(stage)
	downloadCtx, cancelDownload := context.WithTimeout(base, 30*time.Minute)
	stderr, err := m.download(downloadCtx, job, stage, target)
	downloadErr := downloadCtx.Err()
	cancelDownload()
	if err != nil {
		if errors.Is(downloadErr, context.DeadlineExceeded) {
			err = errors.New("The download exceeded 30 minutes. Retry when the connection improves.")
		} else if message := processError(stderr, err); message != "" {
			err = errors.New(message)
		}
		m.finish(job.ID, err)
		return
	}
	if !m.stillDownloading(job.ID) {
		m.finish(job.ID, nil)
		return
	}
	audio, err := findOutput(stage)
	if err != nil {
		m.finish(job.ID, err)
		return
	}
	warnings := []string{}
	sidecars := []string{}
	if job.Settings.EmbedMetadata {
		sidecars, warnings = m.enrich(base, audio, job)
	}
	m.publishAndFinish(job, stage, audio, sidecars, warnings)
}

func (m *Manager) resolveTarget(ctx context.Context, job model.DownloadEntry) (string, error) {
	switch job.Source {
	case "YouTube":
		return "https://www.youtube.com/watch?v=" + job.VideoID, nil
	case "YouTube match for Spotify":
		query := strings.Join(strings.Fields(strings.TrimSpace(job.Artists)+" "+strings.TrimSpace(job.Title)+" official audio"), " ")
		if m.searcher == nil {
			return "ytsearch1:" + query, nil
		}
		match, err := m.searcher.Match(ctx, query)
		if err != nil {
			return "", err
		}
		match = strings.TrimSpace(match)
		if youtubeIDPattern.MatchString(match) {
			return "https://www.youtube.com/watch?v=" + match, nil
		}
		if parsed := strings.TrimPrefix(match, "https://www.youtube.com/watch?v="); parsed != match && youtubeIDPattern.MatchString(parsed) {
			return match, nil
		}
		return "", errors.New("The YouTube search returned an invalid match.")
	case "SoundCloud":
		if m.soundCloud == nil {
			return "", errors.New("Could not look up the SoundCloud track: provider unavailable")
		}
		id := strings.TrimPrefix(job.VideoID, "sc:track:")
		link, err := m.soundCloud.Permalink(ctx, id)
		if err != nil {
			return "", fmt.Errorf("Could not look up the SoundCloud track: %v", err)
		}
		if err := validateSoundCloudURL(link); err != nil {
			return "", err
		}
		return link, nil
	default:
		return "", errors.New("This track can't be downloaded.")
	}
}

func createStaging(job model.DownloadEntry) (string, error) {
	if err := os.MkdirAll(job.Settings.Path, 0o755); err != nil {
		return "", err
	}
	root := filepath.Join(job.Settings.Path, stagingName)
	if err := os.Mkdir(root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("Could not create the temporary download folder: %v", err)
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("The temporary download folder is not a regular directory.")
	}
	stage := filepath.Join(root, sanitizeComponent(job.ID))
	if err := os.Mkdir(stage, 0o700); err != nil {
		return "", fmt.Errorf("Could not prepare this download: %v", err)
	}
	return stage, nil
}

func (m *Manager) download(ctx context.Context, job model.DownloadEntry, stage, target string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	command := exec.Command(m.ytdlp, buildYTDLPArgs(stage, job.Settings.Format, job.Settings.EmbedMetadata, target)...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return "", err
	}
	defer devNull.Close()
	command.Stdin = devNull
	stdout, err := command.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return "", errors.New("Install yt-dlp and FFmpeg to download music.")
		}
		return "", fmt.Errorf("Could not start yt-dlp: %v", err)
	}
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, command.Process.Pid, 10)
	stdoutDone := make(chan string, 1)
	stderrDone := make(chan string, 1)
	go func() { stdoutDone <- readProcessStream(stdout, func(line string) { m.progress(job.ID, line) }) }()
	go func() { stderrDone <- readProcessStream(stderr, nil) }()
	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	var waitErr error
	interrupted := false
	select {
	case waitErr = <-waitDone:
	case <-ctx.Done():
		interrupted = true
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		select {
		case waitErr = <-waitDone:
		case <-time.After(2 * time.Second):
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			waitErr = <-waitDone
		}
	}
	var stderrTail string
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	gotOut, gotErr := false, false
	for !gotOut || !gotErr {
		select {
		case <-stdoutDone:
			gotOut = true
		case stderrTail = <-stderrDone:
			gotErr = true
		case <-deadline.C:
			if interrupted {
				return stderrTail, ctx.Err()
			}
			return stderrTail, waitErr
		}
	}
	if interrupted {
		return stderrTail, ctx.Err()
	}
	return stderrTail, waitErr
}

func readProcessStream(reader io.Reader, lineFn func(string)) string {
	buffered := bufio.NewReaderSize(reader, 4096)
	var tail []byte
	line := make([]byte, 0, 4096)
	flush := func() {
		if lineFn != nil && len(line) > 0 {
			lineFn(string(line))
		}
		line = line[:0]
	}
	chunk := make([]byte, 4096)
	for {
		count, err := buffered.Read(chunk)
		if count > 0 {
			tail = append(tail, chunk[:count]...)
			if len(tail) > 8192 {
				tail = append([]byte(nil), tail[len(tail)-8192:]...)
			}
			for _, value := range chunk[:count] {
				if value == '\n' || value == '\r' {
					flush()
				} else if len(line) < 4096 {
					line = append(line, value)
				}
			}
		}
		if err != nil {
			flush()
			break
		}
	}
	return string(tail)
}

func processError(stderr string, waitErr error) string {
	lines := strings.FieldsFunc(stderr, func(r rune) bool { return r == '\n' || r == '\r' })
	last := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			last = line
		}
	}
	for index := len(lines) - 1; index >= 0; index-- {
		if strings.Contains(lines[index], "ERROR") {
			return strings.TrimSpace(lines[index])
		}
	}
	if last != "" {
		return last
	}
	var exit *exec.ExitError
	if errors.As(waitErr, &exit) {
		return fmt.Sprintf("The download failed (exit code %d).", exit.ExitCode())
	}
	if waitErr != nil {
		return "The download was terminated."
	}
	return ""
}

func (m *Manager) progress(id, line string) {
	value, ok := parseProgress(line)
	if !ok {
		return
	}
	m.mu.Lock()
	index := m.jobIndexLocked(id)
	if index < 0 || m.jobs[index].Status != model.DownloadStatusDownloading || value <= m.jobs[index].Progress {
		m.mu.Unlock()
		return
	}
	m.jobs[index].Progress = value
	now := time.Now()
	if previous := m.progressAt[id]; !previous.IsZero() && now.Sub(previous) < 400*time.Millisecond {
		m.mu.Unlock()
		return
	}
	m.progressAt[id] = now
	if err := m.saveLocked(); err != nil {
		m.log.Warn("download progress persistence failed", "id", id, "error", err)
	}
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	m.emit(snapshot)
}

func (m *Manager) stillDownloading(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	index := m.jobIndexLocked(id)
	return index >= 0 && m.jobs[index].Status == model.DownloadStatusDownloading
}

func (m *Manager) publishAndFinish(job model.DownloadEntry, stage, audio string, sidecars, warnings []string) {
	m.mu.Lock()
	index := m.jobIndexLocked(job.ID)
	if index >= 0 && m.jobs[index].Status == model.DownloadStatusDownloading {
		published, publicationWarnings, runErr := publish(stage, audio, sidecars, job)
		warnings = append(warnings, publicationWarnings...)
		current := &m.jobs[index]
		now := time.Now().Unix()
		current.FinishedAt = &now
		if runErr != nil {
			message := truncateError(runErr.Error())
			current.Status = model.DownloadStatusFailed
			current.Error = &message
			current.Progress = min(current.Progress, 99)
		} else {
			current.Status = model.DownloadStatusCompleted
			current.Progress = 100
			current.FilePath = &published
			current.Error = nil
			current.Warnings = append([]string(nil), warnings...)
		}
		m.pruneLocked()
		if err := m.saveLocked(); err != nil {
			message := err.Error()
			if published != "" {
				message = "Audio saved to your folder. " + message
			}
			message = truncateError(message)
			if currentIndex := m.jobIndexLocked(job.ID); currentIndex >= 0 {
				m.jobs[currentIndex].Status = model.DownloadStatusFailed
				m.jobs[currentIndex].Error = &message
			}
			m.log.Error("download terminal persistence failed", "id", job.ID, "error", err)
		}
	}
	delete(m.running, job.ID)
	delete(m.progressAt, job.ID)
	snapshot := m.snapshotLocked()
	closed := m.closed
	m.mu.Unlock()
	m.emit(snapshot)
	if !closed {
		m.pump()
	}
}

func (m *Manager) finish(id string, runErr error) {
	m.mu.Lock()
	delete(m.running, id)
	delete(m.progressAt, id)
	index := m.jobIndexLocked(id)
	if index >= 0 && m.jobs[index].Status == model.DownloadStatusDownloading && runErr != nil {
		job := &m.jobs[index]
		now := time.Now().Unix()
		message := truncateError(runErr.Error())
		job.FinishedAt = &now
		job.Status = model.DownloadStatusFailed
		job.Error = &message
		job.Progress = min(job.Progress, 99)
		m.pruneLocked()
		if err := m.saveLocked(); err != nil {
			message = truncateError(err.Error())
			if currentIndex := m.jobIndexLocked(id); currentIndex >= 0 {
				m.jobs[currentIndex].Error = &message
			}
			m.log.Error("download terminal persistence failed", "id", id, "error", err)
		}
	}
	snapshot := m.snapshotLocked()
	closed := m.closed
	m.mu.Unlock()
	m.emit(snapshot)
	if !closed {
		m.pump()
	}
}

func truncateError(message string) string {
	runes := []rune(message)
	if len(runes) > 600 {
		return string(runes[:600])
	}
	return message
}

func findOutput(stage string) (string, error) {
	entries, err := os.ReadDir(stage)
	if err != nil {
		return "", err
	}
	output := ""
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			continue
		}
		path := filepath.Join(stage, entry.Name())
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(entry.Name()), "."))
		if strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())) != "audio" || !downloadExtensions[ext] {
			continue
		}
		if output != "" {
			return "", errors.New("The downloader produced ambiguous audio output.")
		}
		output = path
	}
	if output == "" {
		return "", errors.New("The download produced no finished audio file.")
	}
	return output, nil
}

func publish(stage, audio string, candidates []string, job model.DownloadEntry) (string, []string, error) {
	extension := strings.ToLower(strings.TrimPrefix(filepath.Ext(audio), "."))
	destination := finalPath(job, extension)
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", nil, err
	}
	root, err := filepath.EvalSymlinks(job.Settings.Path)
	if err != nil {
		return "", nil, err
	}
	canonicalParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", nil, err
	}
	relative, err := filepath.Rel(root, canonicalParent)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", nil, errors.New("The artist folder points outside your download folder.")
	}
	if file, err := os.Open(audio); err != nil {
		return "", nil, err
	} else {
		err = file.Sync()
		_ = file.Close()
		if err != nil {
			return "", nil, err
		}
	}
	companions := collectSidecars(stage, candidates)
	for _, source := range companions {
		if file, err := os.Open(source); err == nil {
			_ = file.Sync()
			_ = file.Close()
		}
	}
	warnings := []string{}
	for suffix := range 1000 {
		dest := collisionPath(destination, suffix)
		nameStem := strings.TrimSuffix(filepath.Base(dest), filepath.Ext(dest))
		if err := os.Link(audio, dest); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", warnings, fmt.Errorf("Could not save the audio file: %v", err)
		}
		created := []string{dest}
		collision := false
		attemptWarnings := []string{}
		for ext, source := range companions {
			companion := filepath.Join(parent, nameStem+"."+ext)
			if err := os.Link(source, companion); err != nil {
				if errors.Is(err, os.ErrExist) {
					collision = true
					break
				}
				attemptWarnings = append(attemptWarnings, truncateError(fmt.Sprintf("Couldn't save the %s file: %v", companionLabel(ext), err)))
				continue
			}
			created = append(created, companion)
		}
		if collision {
			for _, path := range created {
				_ = os.Remove(path)
			}
			continue
		}
		warnings = append(warnings, attemptWarnings...)
		return dest, warnings, nil
	}
	return "", warnings, errors.New("Too many files share this track's filename. Choose another download folder.")
}
func collisionPath(path string, suffix int) string {
	if suffix == 0 {
		return path
	}
	extension := filepath.Ext(path)
	return strings.TrimSuffix(path, extension) + fmt.Sprintf(" (%d)", suffix) + extension
}

func collectSidecars(stage string, candidates []string) map[string]string {
	found := make(map[string]string)
	for _, path := range candidates {
		if filepath.Dir(path) != stage {
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		if (ext != "jpg" && ext != "png" && ext != "lrc" && ext != "txt") || found[ext] != "" {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			continue
		}
		found[ext] = path
	}
	return found
}

func companionLabel(extension string) string {
	switch extension {
	case "jpg", "png":
		return "cover art"
	case "lrc":
		return "synced lyrics"
	case "txt":
		return "lyrics"
	default:
		return "companion"
	}
}
