// Package logging provides slog handlers for dotman's output.
package logging

import (
	"context"
	"log/slog"
)

// PrefixKey is the attribute that PrefixHandler turns into a message prefix.
const PrefixKey = "pkg"

// PrefixHandler wraps another handler and moves the PrefixKey attribute,
// added with Logger.With, to the front of each message as "name: ".
type PrefixHandler struct {
	inner  slog.Handler
	prefix string

	// grouped is set once WithGroup is called; PrefixKey attributes after
	// that belong to the group and are passed through unchanged.
	grouped bool
}

func NewPrefixHandler(inner slog.Handler) *PrefixHandler {
	return &PrefixHandler{inner: inner}
}

func (h *PrefixHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *PrefixHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.prefix != "" {
		r.Message = h.prefix + ": " + r.Message
	}

	return h.inner.Handle(ctx, r)
}

func (h *PrefixHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h

	if !h.grouped {
		rest := make([]slog.Attr, 0, len(attrs))
		for _, a := range attrs {
			if a.Key == PrefixKey {
				clone.prefix = a.Value.String()
				continue
			}
			rest = append(rest, a)
		}
		attrs = rest
	}

	if len(attrs) > 0 {
		clone.inner = h.inner.WithAttrs(attrs)
	}

	return &clone
}

func (h *PrefixHandler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.inner = h.inner.WithGroup(name)
	clone.grouped = true
	return &clone
}
