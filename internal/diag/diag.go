// Package diag records daemon and client diagnostics in memory and on disk.
package diag

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	fileName        = "ryotunesd.log"
	rotatedName     = "ryotunesd.log.1"
	ringCapacity    = 2000
	fileLimit       = 2 << 20
	messageLimit    = 2000
	targetLimit     = 120
	defaultQueryMax = 500
)

type state struct {
	mu       sync.Mutex
	entries  []model.LogEntry
	path     string
	bytes    int64
	errors   int
	warnings int
}

// Handler is a slog handler backed by the diagnostics ring and rotating file.
type Handler struct {
	state  *state
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

var current atomic.Pointer[Handler]

// New creates a diagnostics handler rooted in logsDir and makes it the package recorder.
func New(logsDir string) (*Handler, error) {
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return nil, fmt.Errorf("create logs folder: %w", err)
	}
	if err := os.Chmod(logsDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure logs folder: %w", err)
	}
	path := filepath.Join(logsDir, fileName)
	info, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect diagnostics log: %w", err)
	}
	st := &state{path: path}
	if err == nil {
		st.bytes = info.Size()
		st.entries = readTail(path, ringCapacity)
		for _, entry := range st.entries {
			switch entry.Level {
			case "error":
				st.errors++
			case "warn":
				st.warnings++
			}
		}
	}
	h := &Handler{state: st, level: LevelFromEnv()}
	current.Store(h)
	return h, nil
}

// LevelFromEnv returns the RYOTUNES_LOG level, defaulting to warn.
func LevelFromEnv() slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("RYOTUNES_LOG"))) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "error":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}

// Enabled reports whether a slog record meets the configured minimum level.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return h != nil && h.state != nil && level >= h.level
}

// Handle records one slog event as a daemon diagnostic.
func (h *Handler) Handle(_ context.Context, record slog.Record) error {
	if h == nil || h.state == nil {
		return nil
	}
	target := slogTarget(record)
	attrs := append([]slog.Attr(nil), h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})
	message := record.Message
	parts := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		attr.Value = attr.Value.Resolve()
		if attr.Key == "target" {
			target = attr.Value.String()
			continue
		}
		key := attr.Key
		if len(h.groups) != 0 {
			key = strings.Join(append(append([]string(nil), h.groups...), key), ".")
		}
		parts = append(parts, key+"="+attr.Value.String())
	}
	if len(parts) != 0 {
		message += " " + strings.Join(parts, " ")
	}
	h.record(levelName(record.Level), "daemon", target, message, record.Time)
	return nil
}

// WithAttrs returns a handler that includes attrs in subsequent records.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(slices.Clone(h.attrs), attrs...)
	return &clone
}

// WithGroup returns a handler that qualifies subsequent attribute names with name.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = append(slices.Clone(h.groups), name)
	return &clone
}

// Query returns a filtered, newest-first diagnostics snapshot.
func Query(level, source, query string, limit int) model.LogsResult {
	h := current.Load()
	if h == nil {
		return model.LogsResult{Entries: []model.LogEntry{}}
	}
	return h.Query(level, source, query, limit)
}

// Query returns a filtered, newest-first diagnostics snapshot.
func (h *Handler) Query(level, source, query string, limit int) model.LogsResult {
	if level == "" {
		level = "warn"
	}
	if source == "" {
		source = "all"
	}
	if limit == 0 {
		limit = defaultQueryMax
	}
	limit = min(max(limit, 1), ringCapacity)
	needle := strings.ToLower(query)

	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	entries := make([]model.LogEntry, 0, min(limit, len(h.state.entries)))
	for index := len(h.state.entries) - 1; index >= 0 && len(entries) < limit; index-- {
		entry := h.state.entries[index]
		if level == "warn" && entry.Level == "info" {
			continue
		}
		if level == "error" && entry.Level != "error" {
			continue
		}
		if source != "all" && entry.Source != source {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(entry.Target), needle) && !strings.Contains(strings.ToLower(entry.Message), needle) {
			continue
		}
		entries = append(entries, entry)
	}
	return model.LogsResult{
		Entries: entries, Total: len(h.state.entries), Errors: h.state.errors,
		Warnings: h.state.warnings, Path: h.state.path, FileBytes: h.state.bytes,
	}
}

// RecordClient records a renderer diagnostic using the package recorder.
func RecordClient(message, target, level string) {
	if h := current.Load(); h != nil {
		h.RecordClient(message, target, level)
	}
}

// RecordClient records a renderer diagnostic.
func (h *Handler) RecordClient(message, target, level string) {
	if target == "" {
		target = "client"
	}
	if level != "warn" {
		level = "error"
	}
	h.record(level, "client", target, message, time.Now())
}

// OpenLogsFolder reveals the diagnostics folder with xdg-open.
func OpenLogsFolder() error {
	h := current.Load()
	if h == nil {
		return fmt.Errorf("Logging is not initialised yet.")
	}
	return h.OpenLogsFolder()
}

// OpenLogsFolder reveals the diagnostics folder with xdg-open.
func (h *Handler) OpenLogsFolder() error {
	dir := filepath.Dir(h.state.path)
	if dir == "." || dir == "" {
		return fmt.Errorf("The log file has no folder.")
	}
	cmd := exec.Command("xdg-open", dir)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Could not open the logs folder: %v", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// CapturePanic records a recovered panic and then resumes unwinding.
func CapturePanic(target string) {
	value := recover()
	if value == nil {
		return
	}
	if h := current.Load(); h != nil {
		h.record("error", "daemon", target, fmt.Sprint(value), time.Now())
	}
	panic(value)
}

func (h *Handler) record(level, source, target, message string, at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}
	entry := model.LogEntry{
		Timestamp: at.UTC().Format("2006-01-02T15:04:05.000Z"),
		Level:     level,
		Source:    source,
		Target:    truncate(cleanLine(target), targetLimit),
		Message:   truncate(cleanLine(message), messageLimit),
	}
	line := formatLine(entry)
	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	switch level {
	case "error":
		h.state.errors++
	case "warn":
		h.state.warnings++
	}
	if len(h.state.entries) == ringCapacity {
		copy(h.state.entries, h.state.entries[1:])
		h.state.entries[len(h.state.entries)-1] = entry
	} else {
		h.state.entries = append(h.state.entries, entry)
	}
	h.state.append(line)
}

func (st *state) append(line string) {
	if st.bytes >= fileLimit || st.bytes > 0 && st.bytes+int64(len(line)) > fileLimit {
		rotated := filepath.Join(filepath.Dir(st.path), rotatedName)
		if err := os.Rename(st.path, rotated); err == nil || os.IsNotExist(err) {
			st.bytes = 0
		}
	}
	file, err := os.OpenFile(st.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	written, writeErr := io.WriteString(file, line)
	if writeErr == nil {
		_ = file.Sync()
		st.bytes += int64(written)
	}
	_ = file.Close()
}

func formatLine(entry model.LogEntry) string {
	return fmt.Sprintf("%s [%s] %s %s: %s\n", entry.Timestamp, strings.ToUpper(entry.Level), entry.Source, entry.Target, entry.Message)
}

func parseLine(line string) (model.LogEntry, bool) {
	if len(line) < 31 {
		return model.LogEntry{}, false
	}
	stampEnd := strings.Index(line, " [")
	if stampEnd < 0 {
		return model.LogEntry{}, false
	}
	stamp := line[:stampEnd]
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", stamp); err != nil {
		return model.LogEntry{}, false
	}
	rest := line[stampEnd+2:]
	levelEnd := strings.Index(rest, "] ")
	if levelEnd < 0 {
		return model.LogEntry{}, false
	}
	level := strings.ToLower(rest[:levelEnd])
	if level != "info" && level != "warn" && level != "error" {
		return model.LogEntry{}, false
	}
	rest = rest[levelEnd+2:]
	originEnd := strings.Index(rest, ": ")
	if originEnd < 0 {
		return model.LogEntry{}, false
	}
	origin := rest[:originEnd]
	space := strings.IndexByte(origin, ' ')
	if space < 0 {
		return model.LogEntry{}, false
	}
	source, target := origin[:space], origin[space+1:]
	if source != "daemon" && source != "client" {
		return model.LogEntry{}, false
	}
	return model.LogEntry{Timestamp: stamp, Level: level, Source: source, Target: target, Message: rest[originEnd+2:]}, true
}

func readTail(path string, limit int) []model.LogEntry {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	entries := make([]model.LogEntry, 0, min(limit, len(lines)))
	for index := max(0, len(lines)-limit); index < len(lines); index++ {
		if entry, ok := parseLine(lines[index]); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func slogTarget(record slog.Record) string {
	if record.PC != 0 {
		if function := runtime.FuncForPC(record.PC); function != nil {
			return function.Name()
		}
	}
	return "daemon"
}

func levelName(level slog.Level) string {
	if level >= slog.LevelError {
		return "error"
	}
	if level >= slog.LevelWarn {
		return "warn"
	}
	return "info"
}

func cleanLine(value string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(value)
}

func truncate(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit-1]) + "…"
}
