package spotify

import (
	"context"
	"fmt"
	"log/slog"

	librespot "github.com/devgianlu/go-librespot"
)

type librespotLogger struct {
	log   *slog.Logger
	attrs []any
}

func (l *librespotLogger) emit(level slog.Level, message string) {
	l.log.Log(context.Background(), level, message, l.attrs...)
}
func (l *librespotLogger) formatted(level slog.Level, format string, args ...interface{}) {
	l.emit(level, fmt.Sprintf(format, args...))
}

// Tracef logs a formatted librespot trace message.
func (l *librespotLogger) Tracef(format string, args ...interface{}) {
	l.formatted(slog.LevelDebug, format, args...)
}

// Debugf logs a formatted librespot debug message.
func (l *librespotLogger) Debugf(format string, args ...interface{}) {
	l.formatted(slog.LevelDebug, format, args...)
}

// Infof logs a formatted librespot information message.
func (l *librespotLogger) Infof(format string, args ...interface{}) {
	l.formatted(slog.LevelInfo, format, args...)
}

// Warnf logs a formatted librespot warning.
func (l *librespotLogger) Warnf(format string, args ...interface{}) {
	l.formatted(slog.LevelWarn, format, args...)
}

// Errorf logs a formatted librespot error.
func (l *librespotLogger) Errorf(format string, args ...interface{}) {
	l.formatted(slog.LevelError, format, args...)
}

// Trace logs a librespot trace message.
func (l *librespotLogger) Trace(args ...interface{}) { l.emit(slog.LevelDebug, fmt.Sprint(args...)) }

// Debug logs a librespot debug message.
func (l *librespotLogger) Debug(args ...interface{}) { l.emit(slog.LevelDebug, fmt.Sprint(args...)) }

// Info logs a librespot information message.
func (l *librespotLogger) Info(args ...interface{}) { l.emit(slog.LevelInfo, fmt.Sprint(args...)) }

// Warn logs a librespot warning.
func (l *librespotLogger) Warn(args ...interface{}) { l.emit(slog.LevelWarn, fmt.Sprint(args...)) }

// Error logs a librespot error.
func (l *librespotLogger) Error(args ...interface{}) { l.emit(slog.LevelError, fmt.Sprint(args...)) }

// WithField returns a logger with one structured attribute.
func (l *librespotLogger) WithField(key string, value interface{}) librespot.Logger {
	return &librespotLogger{log: l.log, attrs: append(append([]any(nil), l.attrs...), key, value)}
}

// WithError returns a logger with an error attribute.
func (l *librespotLogger) WithError(err error) librespot.Logger {
	return l.WithField("error", err)
}
