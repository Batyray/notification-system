package logger

import (
	"io"
	"log/slog"
	"os"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

type Options struct {
	Service     string
	Environment string
	Version     string
	Writer      io.Writer
	Format      Format
	Level       slog.Level
}

type Logger struct {
	*slog.Logger
}

func New(opts Options) *Logger {
	if opts.Service == "" {
		panic("logger: service is required")
	}
	if opts.Environment == "" {
		panic("logger: environment is required")
	}
	if opts.Version == "" {
		panic("logger: version is required")
	}

	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	if opts.Format == "" {
		if opts.Environment == "development" {
			opts.Format = FormatText
		} else {
			opts.Format = FormatJSON
		}
	}

	var handler slog.Handler
	handlerOpts := &slog.HandlerOptions{Level: opts.Level}

	switch opts.Format {
	case FormatText:
		handler = slog.NewTextHandler(opts.Writer, handlerOpts)
	default:
		handler = slog.NewJSONHandler(opts.Writer, handlerOpts)
	}

	base := slog.New(handler).With(
		"service", opts.Service,
		"environment", opts.Environment,
		"version", opts.Version,
	)

	return &Logger{Logger: base}
}

func (l *Logger) With(args ...any) *Logger {
	return &Logger{Logger: l.Logger.With(args...)}
}
