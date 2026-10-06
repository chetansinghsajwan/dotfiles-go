# dotman: Nix-style store

## Goals

1. Everything dotman builds lives in a store.
2. A store path's hash is computed from its inputs, before building.
3. If a store path exists, it is complete: that input has been built.
4. The same input always gives the same output and the same hash.

## Status

Implemented (all migration steps below). Where the implementation differs
from the original plan, this document describes the implementation.

| Piece | Before | Now |
|---|---|---|
| Downloads | `PathForUrl(url, hash)`, unpinned allowed | Fixed-output derivations keyed by name + content hash; unpinned is an error |
| Package outputs | `CreatePath` → `uuid-name`, rebuilt every run | Input-addressed, built once |
| Profile | uuid-named, no generations | A derivation; generations, rollback, GC |
| Package code | `Install(log, cfg, store, storePath)` | `Derive(ev)` returns data; builders build it |

## Core model: evaluation vs. realisation

A package doesn't build itself directly. It first **evaluates** to a
`Derivation`, which is plain, hashable data. A **builder** then realises that
derivation into a store path.

```go
// core/derivation.go. Pure data. Its canonical JSON (with inputs replaced by
// their output paths) is what the store hash is computed from.
type Derivation struct {
    Name     string
    System   string                 // "linux/amd64", "" if it doesn't matter
    Builder  *Builder               // {Name, Hash: <builder source hash>, Build}
    Attrs    any                    // serialized to JSON; the builder sees only that
    Inputs   map[string]*Derivation // deps, fetches, local sources
    Fixed    *FixedOutput           // set for fetches: pins the output's hash
    HostDeps []string               // not hashed; warned about if missing
}

type Package interface {
    Name() string
    // Derive must not do IO beyond hashing local files.
    Derive(ev *Eval) (*Derivation, error)
}

type BuildFunc func(b *Build) error

// b.Out            path to build into (temporary; see "Realisation")
// b.Input("zip")   realised store path of a named input
// b.Decode(&attrs) unmarshal Attrs into the builder's own struct
```

`Realiser` (core/realise.go) computes paths (`Path`), builds (`Realise`) and
verifies (`Check`).

### Rule: builders only see what was hashed

The builder gets `Attrs` back **after a JSON round-trip**, never the original Go
struct, and gets inputs only through `b.Input`. Anything that wasn't hashed
can't reach the build. Go has no sandbox, so this stands in for one: a missed
input breaks the build visibly instead of causing a stale cache hit.

For example, `yazi.Plugin` keeps `src fs.FS` and `repo` in unexported fields
that `json.Marshal` drops. Under this rule they must become explicit inputs.

## Hashing

```
hash = sha256(canonicalJSON(drv with each input replaced by its output path))
path = <store>/<base32(hash)[:32]>-<name>
```

- Inputs are referenced by output path, so when a dependency's hash changes,
  every dependent's hash changes too.
- Attrs are canonicalized (marshal, decode, marshal again), so object keys
  are sorted and struct field order doesn't matter. Numbers keep their text.
- `Build.Decode` turns numbers in `any` values back into `int64` (or
  `float64` if fractional), so a `1920` in a settings map isn't written out
  as `1920.0`.
- **The store root is an input.** Outputs embed absolute paths, so the store
  dir is part of the hash, as in Nix.
- **System** (`GOOS/GOARCH`) is an input.

### Builder code is an input

If `writeWrappers` changes but the config doesn't, the hash has to change
anyway, or dotman silently keeps the old output. This is the main risk in the
whole design.

- Each builder package embeds its own source with
  `//go:embed *.go *.toml` (plus its templates) and `NewBuilder` sets
  `Builder.Hash` to the tree hash of that FS.
- A toolchain hash (`ToolchainHash`) covers the sources of `core/` and
  `lib/`, the Go version, and every third-party module's version and
  checksum from `debug.ReadBuildInfo()`. That replaces embedding `go.sum`, so
  no Go file needs to live at the repo root. It is mixed into every
  derivation that isn't fixed-output.
- `*.go` also matches `_test.go`, so editing a test in one of those packages
  rebuilds its derivations. That is harmless and cheap.

### Global config is resolved during evaluation

`Install(cfg)` currently reads `cfg.Theme` while building. Instead, `Derive`
resolves the theme to its base16 colors and puts **the colors** into `Attrs`.
Editing a theme's colors then changes the hash, not only renaming the theme.

## Everything is a derivation

| Kind | Hash from | Notes |
|---|---|---|
| Fetch (fixed-output) | `name + content hash` only, not the URL | A mirror change doesn't refetch |
| Tree fetch | `name + hash of the extracted tree` | For GitHub source archives, which aren't byte-stable (like Nix's `fetchzip`) |
| Local source | file/dir content | Embedded plugins (`dotman.LocalSource`); copied into the store |
| Package | drv JSON | yazi, zellij, lazygit, pv, op |
| Profile | the package output paths | Merges `bin/`, `share/` |

- **Unpinned downloads are an error.** `FakeHash` stays as the way to discover
  the right hash.
- Fetch helpers live in `lib/fetch.go`: `FetchUrl`, `FetchGithubRelease`
  (flat), and `FetchTarball`, `FetchGithubArchive` (tree).
- Tree hash (`TreeHash`): deterministic walk over sorted paths, hashing each
  entry's path, type, executable bit, and content or symlink target. Other
  mode bits and times are left out, so a tree hashes the same before and
  after normalization, and an `embed.FS` hashes the same as its copy.
- Fixed-output builders may be impure (download, close over an `fs.FS`):
  the output is checked against the pinned hash instead.

## Realisation

```
path = <store>/<hash>-<name>
if exists(path): done
out  = <store>/.tmp-<random>-<name>   # not created; the builder creates it
build(out)
checkSelfRefs(out)    # fail if any file or symlink contains out
normalize(out)        # mtimes, modes, read-only
checkFixed(out)       # fixed-output only: compare against the pinned hash
rename(out, path)     # lost a race and path exists: delete out, done
```

Inputs are realised first, in dependency order. `out` sits in the store's
root, so the rename never moves a (read-only) directory between parents, and
an output can be a single file, as flat fetches are. Any failure removes
`out`.

### Rule: no self-references

Because outputs are built in a temp dir and renamed, an output must not refer
to its own path. References to **inputs** are fine, since inputs are already at
their final paths.

So packages whose parts point at each other are split into separate
derivations:

```
yazi-bin     fetch + extract       -> <h1>-yazi-bin/bin/{yazi,ya}
yazi-config  theme, toml, plugins  -> <h2>-yazi-config/
yazi         wrappers only         -> <h3>-yazi/bin/yazi
               exec <h1>-yazi-bin/bin/yazi
               with YAZI_CONFIG_HOME=<h2>-yazi-config
```

`checkSelfRefs` enforces this: if any file's contents or any symlink target
contains the temp path, the build fails with
`self-reference in bin/yazi; split the derivation`.

This keeps goal 3 exact (existence means complete) with no validity database.
As a bonus, changing a keybind rebuilds only `yazi-config` and the wrapper, and
the binary output stays as it is.

The wrapper derivation is shared: `lib.Wrapper` writes `bin/` scripts whose
`Exec`, `Env` values and `Args` name inputs as `@name@`, replaced with their
store paths at build time. Shell aliases are relative symlinks (`z ->
zellij`), so they don't refer to their own output either.

### Same input, same output

- Set mtimes to the Unix epoch + 1 second (symlinks keep theirs; Go can't
  set them without following the link).
- Normalize modes to `0444` / `0555` (dirs `0555`).
- Make outputs read-only after build. This also catches apps that try to write
  into their config dir (e.g. `ya pkg` writing `package.toml`).
- `dotman build --check` rebuilds into a temp path, tree-hashes both and diffs
  them.

## Store layout

```
~/.local/share/dotman/store/
  3x9k…-yazi/                    package output (read-only)
  7m2p…-yazi-bin/
  q8r1…-yazi-config/
  a1b2…-yazi-x86_64-unknown-linux-musl.zip   fetch (a file), path from content hash
  b7c8…-plugins-7200d73/         tree fetch of a GitHub archive
  c4d5…-places.yazi/             local source, path from content
  e6f7…-profile/
  .tmp-*                         in-progress builds; safe to delete
~/.local/state/dotman/
  profile -> profiles/profile-12-link
  profiles/profile-12-link -> <store>/e6f7…-profile
```

- Each generation link records one profile, so rollback comes for free.
- Generation links are the GC roots.

## Garbage collection

- Mark everything reachable from the generation links. A path's references
  are found by scanning its files and symlinks for store hashes, as Nix does,
  so there's no references database.
- References are found by searching for `<store root>/` followed by a hash,
  so outputs must refer to store paths by absolute path (wrappers and profile
  symlinks do).
- Sweep unreachable store paths and any leftover `.tmp-*` outputs. GC must
  not run during a build, since it would delete that build's `.tmp-*`.
- Fetches and local sources are only needed at build time, so GC deletes
  them; a later rebuild downloads them again, like Nix.
- The profile link is a root too, so a pre-generations profile isn't
  collected before the first `switch`.
- `dotman gc --keep N` first deletes all generations but the newest N and the
  current one.

## Dependencies

- **Packaged deps** are `Inputs`. Wrappers use their absolute store paths.
- **Host deps** (e.g. `ffprobe`, `7zz`) are declared as `HostDeps`, aren't
  hashed, and dotman warns when one isn't on PATH at build time. Don't
  pretend they're pure.
- Script-only packages (pv, op) use `lib.ScriptPackage`.

## CLI

| Command | Does |
|---|---|
| `dotman switch` | Build the profile, add a generation (unless it's unchanged), point `profile` at it. The default command |
| `dotman build [pkg…]` | Realise packages (or the profile) without switching; print their paths |
| `dotman build --check [pkg…]` | Also rebuild every non-fixed derivation in the closure and compare tree hashes |
| `dotman rollback` | Point `profile` at the previous generation |
| `dotman generations` | List generations, marking the current one |
| `dotman gc [--keep N]` | Delete unreachable store paths |
| `dotman show-drv [-r] <pkg>` | Print the hashed JSON, to answer "why did this rebuild?"; `-r` includes inputs |

## Migration

All done:

1. **Core.** `Derivation`, `Eval`, `Build`, canonical hashing, builder
   source hashing, the realise loop, fetch derivations (`FetchUrl`,
   `FetchTarball`). Tests in `core/realise_test.go`.
2. **First packages.** pv and op, via `lib.ScriptPackage`.
3. **Profile.** A derivation; generations, `switch`, `rollback`.
4. **Real packages.** yazi, zellij and lazygit, each split into
   bin/config/wrapper; plugins as tree fetches and local sources; theme
   resolved in `Derive`. Generated configs were checked byte-for-byte
   against the old uuid-built ones.
5. **Hardening.** Read-only outputs, normalization, `build --check`, `gc`.
6. **Cleanup.** `Install`, `CreatePath`, `DownloadFile`, the uuid
   dependency and `lib/platform.go` are gone.

## Open questions

1. **Host deps:** package heavy tools like ffmpeg, or keep them as declared
   `HostDeps`? Default: `HostDeps`.
2. **Sharing between machines:** if store paths should ever be copied between
   machines, the store root has to be fixed (like `/nix/store`) rather than
   under `$HOME`. Default: under `$HOME`, no sharing.
3. **Mutable state:** if any tool refuses to run with a read-only config dir,
   it needs an escape hatch (e.g. a wrapper that copies config to a writable
   location). The tools start (`--version`), but day-to-day use hasn't been
   checked yet.
