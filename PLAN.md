# dotman: Nix-style store

## Goals

1. Everything dotman builds lives in a store.
2. A store path's hash is computed from its inputs, before building.
3. If a store path is registered, it is complete: that input has been built.
4. The same input always gives the same output and the same hash.

## Status

Migration steps 1–6 are implemented; step 7 (recorded references) is
planned. This document describes the whole design. Until step 7 lands, the
code differs from it in three places: "Realisation" (every output is built
in a temp dir and renamed, and no output may refer to itself), "References"
(found by scanning at GC time, not recorded), and goal 3 (a path is complete
if it exists).

| Piece | Before | Now |
|---|---|---|
| Downloads | `PathForUrl(url, hash)`, hashes pinned by hand or not at all | Fixed-output derivations keyed by name + content hash; hashes kept in `dotman.lock` |
| Package outputs | `CreatePath` → `uuid-name`, rebuilt every run | Input-addressed, built once |
| Profile | uuid-named, no generations | A derivation; generations, rollback, GC |
| Package code | `Install(log, cfg, store, storePath)` | `Derive(ev)` returns data; builders build it |
| References | none | Recorded at build time in `.meta/` (planned; scanned at GC time today) |

## Core model: evaluation vs. realisation

A package doesn't build itself directly. It first **evaluates** to a
`Derivation`, which is plain, hashable data. A **builder** then realises that
derivation into a store path.

```go
// core/derivation.go. Pure data. Its canonical JSON (with inputs replaced by
// their output paths) is what the store hash is computed from.
type Derivation struct {
    Name        string
    System      string                 // "linux/amd64", "" if it doesn't matter
    Builder     *Builder               // {Name, Hash: <builder source hash>, Build}
    Attrs       any                    // serialized to JSON; the builder sees only that
    Inputs      map[string]*Derivation // deps, fetches, local sources
    RuntimeRefs []*Derivation          // inputs the output uses that scanning can't see
    Fixed       *FixedOutput           // set for fetches: pins the output's hash
    HostDeps    []string               // not hashed; warned about if missing
}

type Package interface {
    Name() string
    // Derive must not do IO beyond hashing local files.
    Derive(ev *Eval) (*Derivation, error)
}

type BuildFunc func(b *Build) error

// b.Out            path to build into: the store path itself, or a temporary
//                  path for fixed outputs (see "Realisation")
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
- `RuntimeRefs` must each be one of `Inputs`, so they are already realised
  and hashed; they add no hashing rule of their own.

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

- Fetch helpers live in `lib/fetch.go`: `FetchUrl`, `FetchGithubRelease`
  (flat), and `FetchTarball`, `FetchGithubArchive` (tree).
- Tree hash (`TreeHash`): deterministic walk over sorted paths, hashing each
  entry's path, type, executable bit, and content or symlink target. Other
  mode bits and times are left out, so a tree hashes the same before and
  after normalization, and an `embed.FS` hashes the same as its copy.
- Fixed-output builders may be impure (download, close over an `fs.FS`):
  the output is checked against its pinned or locked hash instead.

## The lock

Users never write hashes. Packages say only what to fetch (a version, a
revision), and dotman keeps the hashes in `dotman.lock`, at the repo root
and committed, like `go.sum`:

```json
{
  "version": 1,
  "fetches": {
    "https://github.com/sxyazi/yazi/releases/download/v26.9.1/yazi-x86_64-unknown-linux-musl.zip": {
      "hash": "sha256:9b9c…",
      "mode": "flat"
    }
  }
}
```

- A fetch's `FixedOutput` has a `Key` (its url) and no `Hash`. The realiser
  looks the key up in the lock (`core/lock.go`).
- **Not locked yet:** the realiser fetches it while computing its path,
  hashes the output, stores it at the path that hash gives, and records the
  hash. This is trust on first use.
- **Locked:** the path comes from the locked hash. If the path is missing
  (new machine, after `gc`), the fetch runs and must match, or the build fails
  with a hash mismatch and a hint to run `dotman update`.
- **Every system:** packages derive for any `Eval.System` without IO, so
  dotman can list the fetches of all four systems. `switch` prunes entries no
  system uses (e.g. old versions). `update` forgets and re-locks a package's
  entries for every system the lock covered. `lock --all-systems` fills in
  every system up front.
- `mode` (flat or recursive) is part of an entry, so a url locked as a file
  isn't trusted as a tree.
- `LocalSource` still pins its hash directly, computed from the embedded
  files at eval time; it never goes in the lock.
- The lock is saved even when a run fails, since the hashes it recorded are
  still right. Writes are atomic, and keys are sorted so diffs are clean.

## Realisation

A path is complete only once it is **registered**: its refs file
(`<store>/.meta/<hash>-<name>.refs`, see "References") exists. The file is
written last and atomically, so a crash mid-build never leaves a path that
looks complete. Inputs are realised first, in dependency order. Any failure
removes the partial output.

### Input-addressed outputs: built in place

Their path is known before building, so the builder writes straight to it:

```
path = <store>/<hash>-<name>
if registered(path): done
lock(path)                 # flock .meta/<hash>-<name>.lock
if registered(path): done  # another run built it while we waited
remove(path)               # leftovers of a failed build
build(path)
normalize(path)            # mtimes, modes, read-only
register(path, refs(path))
unlock(path)
```

The lock keeps two runs from building the same path at once.

### Fixed outputs: built in a temp dir

Their path comes from their content hash, known only after the build:

```
out = <store>/.tmp-<random>-<name>   # not created; the builder creates it
build(out)
checkSelfRefs(out)    # fail if any file or symlink contains out
normalize(out)
checkFixed(out)       # compare against the pinned or locked hash
rename(out, path)     # lost a race and path exists: delete out
register(path, refs(path))
```

`out` sits in the store's root, so the rename never moves a (read-only)
directory between parents, and an output can be a single file, as flat
fetches are.

### Self-references

An input-addressed output may refer to its own path, e.g. a binary compiled
with `--prefix=$out`, since it is built in place.

A fixed output must not: it is renamed after building, so a reference to
`out` would dangle. `checkSelfRefs` fails the build with
`self-reference in <file>; split the derivation`. Fixed outputs are fetches
and local sources, which don't refer to themselves.

Packages are still split into parts that point at each other, because a
change then rebuilds only the part it touches:

```
yazi-bin     fetch + extract       -> <h1>-yazi-bin/bin/{yazi,ya}
yazi-config  theme, toml, plugins  -> <h2>-yazi-config/
yazi         wrappers only         -> <h3>-yazi/bin/yazi
               exec <h1>-yazi-bin/bin/yazi
               with YAZI_CONFIG_HOME=<h2>-yazi-config
```

Changing a keybind rebuilds only `yazi-config` and the wrapper, and the
binary output stays as it is.

The wrapper derivation is shared: `lib.Wrapper` writes `bin/` scripts whose
`Exec`, `Env` values and `Args` name inputs as `@name@`, replaced with their
store paths at build time. Shell aliases are relative symlinks (`z ->
zellij`).

### Same input, same output

- Set mtimes to the Unix epoch + 1 second (symlinks keep theirs; Go can't
  set them without following the link).
- Normalize modes to `0444` / `0555` (dirs `0555`).
- Make outputs read-only after build. This also catches apps that try to write
  into their config dir (e.g. `ya pkg` writing `package.toml`).
- `dotman build --check` rebuilds into a temp path, tree-hashes both and diffs
  them.

## References

A path's references are the store paths its output uses at runtime. GC keeps
everything the roots reach through them. They are found once, when the path
is built, and recorded; store paths never change, so neither do their
references.

```
refs = scan(output, storePaths(inputClosure)) ∪ drv.RuntimeRefs
```

- **Scan.** Search the output's files and symlink targets for
  `<store root>/` followed by a hash, as Nix does, keeping only hashes of the
  input closure's paths. An output can only refer to what existed when it was
  built, and stray hash-like strings can't match. Outputs must therefore
  refer to store paths by absolute path (wrappers and profile symlinks do).
  Relative references are missed, which is fine because dotman writes them
  only within one path, like the alias `z -> zellij`.
- **Declare.** The scan sees only bytes stored as is. A compressed or packed
  binary (UPX, a zip or jar), UTF-16 strings, or a path assembled at runtime
  hides a reference. The derivation lists such inputs in `RuntimeRefs`.
- **Record.** `.meta/<hash>-<name>.refs` lists the referenced names, one per
  line. Writing it is what registers the path (see "Realisation").
- **API.** `Store.Register(name, refs)`, `Store.Refs(name)` and
  `Store.Valid(name)` in `core/store.go`, so the storage behind them can
  change without touching the realiser or GC.

### Why not…

- **SQLite or DuckDB.** DuckDB is an analytics engine (cgo, columnar, poor
  at small concurrent writes), the opposite of this workload. SQLite works,
  but costs cgo (`mattn/go-sqlite3`) or a large pure-Go module
  (`modernc.org/sqlite`) for a store of tens to hundreds of paths. Nix needs
  it at hundreds of thousands of paths with a daemon and reverse lookups.
  Per-path files are atomic per path, can't corrupt one another, and can be
  read with `cat`. If reverse lookups or scale ever need a database, it goes
  behind the `Store` API.
- **`Inputs` as references, without scanning.** Inputs are what a build
  reads, not what the output uses. The `-bin` derivations of yazi, zellij
  and lazygit take the downloaded `archive` as an input and never use it
  afterwards, so every live generation would keep its archives, and later its
  compilers and source trees. Inputs bound the scan instead.
- **Declared references only.** Forgetting one makes GC delete a path still
  in use. Scanning is the safe default; declarations cover what it can't see.
- **Scanning at GC time.** It reads every byte of every live path on every
  `gc`, and the result never changes.

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
  .tmp-*                         in-progress fixed-output builds; safe to delete
  .meta/
    3x9k…-yazi.refs              references; its existence registers the path
    3x9k…-yazi.lock              held while the path is being built
~/.local/state/dotman/
  profile -> profiles/profile-12-link
  profiles/profile-12-link -> <store>/e6f7…-profile
```

- Each generation link records one profile, so rollback comes for free.
- Generation links are the GC roots.

## Garbage collection

- Mark everything reachable from the roots by following refs files. GC
  never reads output contents.
- Sweep unreachable store paths with their `.meta` files, unregistered
  paths (failed builds), and leftover `.tmp-*` outputs. GC must not run
  during a build, since it would delete that build's partial output.
- Fetches and local sources are only needed at build time, so GC deletes
  them; a later rebuild downloads them again, like Nix.
- The profile link is a root too, so a pre-generations profile isn't
  collected before the first `switch`.
- `dotman gc --keep N` first deletes all generations but the newest N and the
  current one.

## Dependencies

- **Packaged deps** are `Inputs`. Wrappers use their absolute store paths.
- **Runtime deps the scan can't see** are also listed in `RuntimeRefs`.
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
| `dotman lock [--all-systems]` | Fetch and lock everything not locked yet; prune unused entries |
| `dotman update [pkg…]` | Forget and re-lock packages' downloads, accepting upstream changes |

## Migration

Done:

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

Planned:

7. **Recorded references.** The `Store` metadata API; registering paths at
   build time; building input-addressed outputs in place under a lock, with
   `checkSelfRefs` kept for fixed outputs only; `RuntimeRefs`; GC over refs
   files. Paths built before this have no refs file: on first run, scan each
   one as GC does today and register it, rather than rebuild it.

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
