// Kunhua Huang 2026

package logger

import (
	"log/slog"
	"os"
)

// Logger is the unified logging interface for RPCinGo.
// Users can substitute any implementation (zap, zerolog, etc.) by satisfying this interface.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
}

// New returns the default Logger backed by log/slog (text format, stderr, INFO level).
func New() Logger {
	return &slogLogger{slog.New(slog.NewTextHandler(os.Stderr, nil))}
}

// NewWithLevel returns a default Logger with a custom minimum log level.
func NewWithLevel(level slog.Level) Logger {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	return &slogLogger{slog.New(h)}
}

// Nop returns a logger that discards all output. Useful for tests or silent components.
func Nop() Logger { return nopLogger{} }

// slogLogger wraps *slog.Logger to satisfy Logger.
type slogLogger struct{ l *slog.Logger }

func (s *slogLogger) Debug(msg string, args ...any) { s.l.Debug(msg, args...) }
func (s *slogLogger) Info(msg string, args ...any)  { s.l.Info(msg, args...) }
func (s *slogLogger) Warn(msg string, args ...any)  { s.l.Warn(msg, args...) }
func (s *slogLogger) Error(msg string, args ...any) { s.l.Error(msg, args...) }
func (s *slogLogger) With(args ...any) Logger       { return &slogLogger{s.l.With(args...)} }

// nopLogger discards all log output.
type nopLogger struct{}

func (nopLogger) Debug(_ string, _ ...any) {}
func (nopLogger) Info(_ string, _ ...any)  {}
func (nopLogger) Warn(_ string, _ ...any)  {}
func (nopLogger) Error(_ string, _ ...any) {}
func (nopLogger) With(_ ...any) Logger     { return nopLogger{} }
