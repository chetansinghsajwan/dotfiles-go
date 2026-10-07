// Package logging provides per-module loggers for dotman, like Python's
// logging.getLogger. Each package declares its own logger:
//
//	var log = logging.Get("pkg.yazi")
//
// Names are dotted; a module without its own level inherits the level of its
// nearest configured parent ("pkg.yazi" falls back to "pkg", then the root).
// Levels are set with Configure, usually from DOTMAN_LOG, or SetLevel.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// config is replaced as a whole whenever it changes, so loggers created
// before Configure pick up the new handler and levels on their next call.
type config struct {
	base   slog.Handler
	root   slog.Level
	levels map[string]slog.Level

	// Resolved levels by module name.
	cache sync.Map
}

var (
	current atomic.Pointer[config]

	// Serializes Configure and SetLevel.
	mu sync.Mutex
)

func init() {
	// Until Configure is called, log at info to stderr, like Python's
	// last-resort handler.
	current.Store(&config{
		base:   slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: minLevel}),
		root:   slog.LevelInfo,
		levels: map[string]slog.Level{},
	})
}

// Get returns the logger for module name, which prefixes its messages with
// "name: ". It's safe to call from package-level variables, before Configure.
// "" is the root logger, without a prefix.
func Get(name string) *slog.Logger {
	return slog.New(&moduleHandler{name: name})
}

// Configure sends every module's records to base, filtered by level unless a
// module's level is set in spec, and makes the root logger slog's default.
//
// spec is a comma-separated list of levels, e.g. "info,pkg.yazi=debug". A
// bare level overrides level for the root; "name=level" sets name's level.
// Levels are parsed by slog.Level.UnmarshalText, e.g. "debug" or "warn+2".
func Configure(base slog.Handler, level slog.Level, spec string) error {
	root, levels, err := parseSpec(level, spec)
	if err != nil {
		return err
	}

	mu.Lock()
	current.Store(&config{base: base, root: root, levels: levels})
	mu.Unlock()

	slog.SetDefault(Get(""))
	return nil
}

// SetLevel sets the level of module name and, unless they have their own,
// its children's. "" sets the root's.
func SetLevel(name string, level slog.Level) {
	mu.Lock()
	defer mu.Unlock()

	old := current.Load()
	next := &config{base: old.base, root: old.root, levels: make(map[string]slog.Level, len(old.levels)+1)}
	for k, v := range old.levels {
		next.levels[k] = v
	}

	if name == "" {
		next.root = level
	} else {
		next.levels[name] = level
	}

	current.Store(next)
}

func parseSpec(root slog.Level, spec string) (slog.Level, map[string]slog.Level, error) {
	levels := map[string]slog.Level{}

	for part := range strings.SplitSeq(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		name, text, ok := strings.Cut(part, "=")
		if !ok {
			name, text = "", part
		}
		name = strings.TrimSpace(name)

		var level slog.Level
		if err := level.UnmarshalText([]byte(strings.TrimSpace(text))); err != nil {
			return 0, nil, fmt.Errorf("logging: bad level in %q: %w", part, err)
		}

		if name == "" {
			root = level
		} else {
			levels[name] = level
		}
	}

	return root, levels, nil
}

// levelFor returns the level of name's nearest configured ancestor,
// including itself, or the root's.
func (c *config) levelFor(name string) slog.Level {
	if l, ok := c.cache.Load(name); ok {
		return l.(slog.Level)
	}

	level := c.root
	for n := name; n != ""; {
		if l, ok := c.levels[n]; ok {
			level = l
			break
		}

		i := strings.LastIndexByte(n, '.')
		if i < 0 {
			break
		}
		n = n[:i]
	}

	c.cache.Store(name, level)
	return level
}

// moduleHandler looks up the current config on every call, so it can be
// created before Configure.
type moduleHandler struct {
	name string

	// WithAttrs and WithGroup calls, replayed onto the base handler in order.
	ops []func(slog.Handler) slog.Handler
}

func (h *moduleHandler) Enabled(ctx context.Context, level slog.Level) bool {
	c := current.Load()
	return level >= c.levelFor(h.name) && c.base.Enabled(ctx, level)
}

func (h *moduleHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.name != "" {
		r.Message = h.name + ": " + r.Message
	}

	base := current.Load().base
	for _, op := range h.ops {
		base = op(base)
	}

	return base.Handle(ctx, r)
}

func (h *moduleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	return h.with(func(base slog.Handler) slog.Handler { return base.WithAttrs(attrs) })
}

func (h *moduleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	return h.with(func(base slog.Handler) slog.Handler { return base.WithGroup(name) })
}

func (h *moduleHandler) with(op func(slog.Handler) slog.Handler) *moduleHandler {
	ops := make([]func(slog.Handler) slog.Handler, len(h.ops), len(h.ops)+1)
	copy(ops, h.ops)
	return &moduleHandler{name: h.name, ops: append(ops, op)}
}
