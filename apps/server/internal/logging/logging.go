// Package logging configura o log estruturado (JSON) com rotação de arquivos
// em %LOCALAPPDATA%\SacraLink\logs.
package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Options configura o logger.
type Options struct {
	Dir     string
	Level   string // debug | info | warn | error
	Console bool   // também escrever em texto no stderr
}

// ParseLevel converte o nome do nível.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelInfo
}

// fanout envia cada registro a vários handlers.
type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			errs = append(errs, h.Handle(ctx, r.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (f fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (f fanout) WithGroup(name string) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(name)
	}
	return out
}

// Setup cria o logger. O arquivo server.log é rotacionado a cada 10 MB,
// mantendo 10 arquivos por até 30 dias (política de retenção de logs).
func Setup(o Options) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return nil, nil, err
	}
	rot := &lumberjack.Logger{Filename: filepath.Join(o.Dir, "server.log"), MaxSize: 10, MaxBackups: 10, MaxAge: 30, Compress: true}
	opts := &slog.HandlerOptions{Level: ParseLevel(o.Level)}
	var h slog.Handler = slog.NewJSONHandler(rot, opts)
	if o.Console {
		h = fanout{h, slog.NewTextHandler(os.Stderr, opts)}
	}
	return slog.New(h), rot, nil
}
