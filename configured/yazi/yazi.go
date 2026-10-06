package yazi

import (
	"embed"

	"dotman/pkg/yazi"
)

//go:embed init.lua
var initLua string

//go:embed plugins
var plugins embed.FS

var Yazi = yazi.Package{
	Version: "v26.9.1",

	// 7zz and ffprobe feed the properties panel. pv and op, which the
	// previewers and opener run, come from the profile.
	HostDeps: []string{"7zz", "ffprobe"},

	Plugins: []yazi.Plugin{
		yazi.NewPlugin("yazi-rs/plugins:full-border", "7200d73"),
		yazi.NewPlugin("yazi-rs/plugins:toggle-pane", "7200d73"),
		yazi.NewPlugin("yazi-rs/plugins:piper", "7200d73"),
		yazi.NewPlugin("dedukun/bookmarks", "9ef1254"),

		yazi.NewLocalPlugin(plugins, "plugins/properties.lua"),
		yazi.NewLocalPlugin(plugins, "plugins/places.lua"),
		yazi.NewLocalPlugin(plugins, "plugins/linemode-toggle.yazi"),
	},

	InitLua: initLua,

	Settings: yazi.Settings{
		"mgr": map[string]any{
			"ratio":    []int{0, 3, 6},
			"linemode": "perm_time",
		},

		// yazi's defaults (600x900) cap the cached preview image below the
		// preview pane's pixel area on most displays. Requires
		// `ya cache clear` to apply to already-cached previews.
		"preview": map[string]any{
			"max_width":  1920,
			"max_height": 1200,
		},

		// yazi's default open rules already route text to the "edit"
		// opener, so overriding what "edit" runs is enough. %s expands to
		// the (already shell-quoted) target; without spread = true yazi
		// runs one invocation per file.
		"opener": map[string]any{
			"edit": []map[string]any{
				{"run": "op %s", "block": true, "desc": "Edit"},
			},
		},

		"plugin": map[string]any{
			"prepend_previewers": []map[string]any{
				// pv routes CSV/TSV to tidy-viewer and other text to bat.
				{"mime": "text/*", "run": `piper -- pv "$1"`},
				// JSON isn't text/*, so it needs its own rule to skip yazi's
				// jq previewer.
				{"mime": "application/{json,ndjson}", "run": `piper -- pv "$1"`},
			},

			// Background metadata for the properties panel.
			"prepend_fetchers": []map[string]any{
				{
					// yazi's mime sniffer drops the "x-" prefix; both forms
					// are listed since that's undocumented.
					"mime":  "application/{zip,tar,x-tar,7z-compressed,x-7z-compressed,gzip,x-gzip,bzip,bzip2,x-bzip,x-bzip2,xz,x-xz,zstd,rar,x-rar,x-rar-compressed,vnd.rar}",
					"run":   "properties archive",
					"group": "properties-archive",
				},
				{"mime": "text/*", "run": "properties csv", "group": "properties-csv"},
				{"mime": "image/*", "run": "properties image", "group": "properties-image"},
				{"mime": "video/*", "run": "properties media", "group": "properties-media"},
				{"mime": "audio/*", "run": "properties media", "group": "properties-media"},
			},
		},
	},

	Keybinds: []yazi.Keybind{
		{Keys: []string{"p", "p"}, Command: "plugin toggle-pane min-preview", Desc: "Toggle the preview pane"},
		{Keys: []string{"p", "q"}, Command: "plugin places toggle", Desc: "Toggle the quickbar (favorites/bookmarks/drives/recents)"},
		{Keys: []string{"p", "m"}, Command: "plugin properties toggle", Desc: "Toggle the file metadata panel"},
		{Keys: []string{"b", "s"}, Command: "plugin bookmarks save", Desc: "Save current position as a bookmark"},
		{Keys: []string{"'"}, Command: "plugin bookmarks jump", Desc: "Jump to a bookmark"},
		{Keys: []string{"b", "d"}, Command: "plugin bookmarks delete", Desc: "Delete a bookmark"},
		{Keys: []string{"b", "D"}, Command: "plugin bookmarks delete_all", Desc: "Delete all bookmarks"},
		{Keys: []string{"?"}, Command: "help", Desc: "Open help"},

		// "prev"/"next" wrap around at the ends; yazi's default numeric
		// offsets clamp instead.
		{Keys: []string{"{"}, Command: "tab_swap prev", Desc: "Swap current tab with previous tab"},
		{Keys: []string{"}"}, Command: "tab_swap next", Desc: "Swap current tab with next tab"},

		// Moves "new tab" from t t to t n and adds t q to close a tab.
		{Keys: []string{"t", "n"}, Command: "tab_create --current", Desc: "Create a new tab in CWD"},
		{Keys: []string{"t", "q"}, Command: "close", Desc: "Close the current tab"},
		{Keys: []string{"t", "t"}, Command: "noop"},

		// Independent linemode toggles, combined into the linemodes init.lua
		// generates, replacing yazi's one-at-a-time "m" modes.
		{Keys: []string{"m", "p"}, Command: "plugin linemode-toggle toggle_perm", Desc: "Toggle permissions in the linemode"},
		{Keys: []string{"m", "t"}, Command: "plugin linemode-toggle toggle_time", Desc: "Toggle time in the linemode"},
		{Keys: []string{"m", "o"}, Command: "plugin linemode-toggle toggle_owner", Desc: "Toggle owner in the linemode"},
		{Keys: []string{"m", "s"}, Command: "plugin linemode-toggle toggle_size", Desc: "Toggle size in the linemode"},
		{Keys: []string{"m", "b"}, Command: "noop"},
		{Keys: []string{"m", "m"}, Command: "noop"},
		{Keys: []string{"m", "n"}, Command: "noop"},
	},
}
