package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// configure points every module at a text handler without timestamps and
// returns its output.
func configure(t *testing.T, level slog.Level, spec string) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	base := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: minLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})

	if err := Configure(base, level, spec); err != nil {
		t.Fatal(err)
	}

	return &buf
}

func TestHierarchy(t *testing.T) {
	buf := configure(t, slog.LevelWarn, "pkg=info,pkg.yazi=debug")

	Get("core").Info("core info")
	Get("pkg").Debug("pkg debug")
	Get("pkg").Info("pkg info")
	Get("pkg.zellij").Info("zellij info")
	Get("pkg.yazi.plugin").Debug("plugin debug")
	Get("pkgx").Info("pkgx info")

	got := buf.String()
	for _, want := range []string{"pkg: pkg info", "pkg.zellij: zellij info", "pkg.yazi.plugin: plugin debug"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"core info", "pkg debug", "pkgx info"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unexpected %q in:\n%s", unwanted, got)
		}
	}
}

func TestLoggerBeforeConfigure(t *testing.T) {
	log := Get("early").With("k", "v").WithGroup("g")

	buf := configure(t, slog.LevelInfo, "")
	log.Info("hello", "a", 1)

	if want := `msg="early: hello" k=v g.a=1`; !strings.Contains(buf.String(), want) {
		t.Errorf("got %q, want it to contain %q", buf.String(), want)
	}
}

func TestSetLevel(t *testing.T) {
	buf := configure(t, slog.LevelInfo, "")
	log := Get("a.b")

	log.Debug("before")
	SetLevel("a", slog.LevelDebug)
	log.Debug("after")

	got := buf.String()
	if strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("got:\n%s", got)
	}
}

func TestParseSpec(t *testing.T) {
	root, levels, err := parseSpec(slog.LevelInfo, " debug , core = WARN,pkg.yazi=error+1 ")
	if err != nil {
		t.Fatal(err)
	}

	if root != slog.LevelDebug {
		t.Errorf("root = %v, want DEBUG", root)
	}
	if levels["core"] != slog.LevelWarn || levels["pkg.yazi"] != slog.LevelError+1 {
		t.Errorf("levels = %v", levels)
	}

	if _, _, err := parseSpec(slog.LevelInfo, "core=loud"); err == nil {
		t.Error("want an error for a bad level")
	}
}
