package logging

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

// NewHandler returns dotman's handler writing records at or above level to
// f as "2006-01-02 15-04-05:000 [INF] pkg: msg k=v", coloured when f is a
// terminal and NO_COLOR isn't set.
func NewHandler(f *os.File, level slog.Level) slog.Handler {
	return NewPrefixHandler(tint.NewHandler(f, &tint.Options{
		Level:       level,
		NoColor:     !useColor(f),
		ReplaceAttr: replaceAttr,
	}))
}

// replaceAttr formats the built-in time and level attributes. It checks the
// value types so user attributes that happen to be named "time" or "level"
// are left alone.
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
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
