package logging

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lmittmann/tint"
)

// NewHandler returns dotman's handler writing records to f as
// "2006-01-02 15-04-05:000 [INF] pkg: msg k=v", coloured when f is a terminal
// and NO_COLOR isn't set. It writes every level; Configure filters them per
// module. Paths under storeRoot in attribute values and errors are shortened
// to "store:<relative path>"; "" keeps them whole.
func NewHandler(f *os.File, storeRoot string) slog.Handler {
	var r attrReplacer
	if storeRoot != "" {
		r.storePrefix = filepath.Clean(storeRoot) + string(filepath.Separator)
	}

	return tint.NewHandler(f, &tint.Options{
		Level:       minLevel,
		NoColor:     !useColor(f),
		ReplaceAttr: r.replaceAttr,
	})
}

// minLevel is below every level, so the handlers it's set on write all
// records and leave filtering to the module levels.
const minLevel = slog.Level(math.MinInt)

// storeTag marks a path as relative to the store root, e.g.
// "store:<id>-yazi/bin/yazi". It's highlighted in bold bright cyan, ending
// with "22;39" to reset only bold and the foreground so the rest of the value
// keeps its style. tint strips the escapes when colour is off.
const storeTag = "\u001b[1;96mstore:\u001b[22;39m"

type attrReplacer struct {
	// Replaced with storeTag in attribute values; "" disables it.
	storePrefix string
}

// replaceAttr shortens store paths and formats the built-in time and level
// attributes. It checks the value types so user attributes that happen to be
// named "time" or "level" are left alone.
func (r attrReplacer) replaceAttr(groups []string, a slog.Attr) slog.Attr {
	a = r.trimStore(a)

	if len(groups) > 0 {
		return a
	}

	switch a.Key {
	case slog.TimeKey:
		if a.Value.Kind() == slog.KindTime {
			return slog.String(a.Key, formatTime(a.Value.Time()))
		}
	case slog.LevelKey:
		if level, ok := a.Value.Any().(slog.Level); ok {
			return formatLevel(level)
		}
	}

	return a
}

// trimStore replaces the store prefix with storeTag in a's value if it's a
// string or an error, keeping errors as errors.
func (r attrReplacer) trimStore(a slog.Attr) slog.Attr {
	if r.storePrefix == "" {
		return a
	}

	switch a.Value.Kind() {
	case slog.KindString:
		if s := a.Value.String(); strings.Contains(s, r.storePrefix) {
			return slog.String(a.Key, strings.ReplaceAll(s, r.storePrefix, storeTag))
		}
	case slog.KindAny:
		if err, ok := a.Value.Any().(error); ok && strings.Contains(err.Error(), r.storePrefix) {
			return slog.Any(a.Key, errors.New(strings.ReplaceAll(err.Error(), r.storePrefix, storeTag)))
		}
	}

	return a
}

// formatTime formats t as "2006-01-02 15-04-05:000"; time.Format only puts
// fractional seconds after '.' or ',', so the milliseconds are added here.
func formatTime(t time.Time) string {
	return fmt.Sprintf("%s:%03d", t.Format("2006-01-02 15-04-05"), t.Nanosecond()/int(time.Millisecond))
}

// ANSI colours for levels, matching tint's defaults.
const (
	colorBrightRed    = 9
	colorBrightGreen  = 10
	colorBrightYellow = 11
)

func formatLevel(level slog.Level) slog.Attr {
	switch {
	case level < slog.LevelInfo:
		return slog.String(slog.LevelKey, "[DBG]")
	case level < slog.LevelWarn:
		return tint.Attr(colorBrightGreen, slog.String(slog.LevelKey, "[INF]"))
	case level < slog.LevelError:
		return tint.Attr(colorBrightYellow, slog.String(slog.LevelKey, "[WRN]"))
	default:
		return tint.Attr(colorBrightRed, slog.String(slog.LevelKey, "[ERR]"))
	}
}

func useColor(f *os.File) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}

	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
