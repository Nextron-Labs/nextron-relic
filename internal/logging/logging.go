// Package logging configures structured logging (log/slog) for the console
// and the collection log stored inside the archive.
package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// ParseLevel parses debug, info, warn or error.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("invalid log level %q (debug, info, warn, error)", s)
}

// NewConsole returns a logger writing text or JSON lines to w.
func NewConsole(w io.Writer, format string, level slog.Level) (*slog.Logger, error) {
	opts := &slog.HandlerOptions{Level: level}
	switch format {
	case "text", "":
		return slog.New(slog.NewTextHandler(w, opts)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(w, opts)), nil
	}
	return nil, fmt.Errorf("invalid log format %q (text, json)", format)
}

// NewFileHandler returns a JSON lines handler for log files (the archive's
// relic.log and --log-file). Timestamps are written in UTC, like all other
// timestamps in the archive and the result.
func NewFileHandler(w io.Writer, level slog.Leveler) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
				a.Value = slog.TimeValue(a.Value.Time().UTC())
			}
			return a
		},
	})
}

// Tee returns a logger that writes to l and additionally to h.
func Tee(l *slog.Logger, h slog.Handler) *slog.Logger {
	return slog.New(multi{l.Handler(), h})
}

// Discard is a logger that drops everything.
func Discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type multi []slog.Handler

func (m multi) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range m {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (m multi) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range m {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (m multi) WithAttrs(a []slog.Attr) slog.Handler {
	out := make(multi, len(m))
	for i, h := range m {
		out[i] = h.WithAttrs(a)
	}
	return out
}

func (m multi) WithGroup(name string) slog.Handler {
	out := make(multi, len(m))
	for i, h := range m {
		out[i] = h.WithGroup(name)
	}
	return out
}
