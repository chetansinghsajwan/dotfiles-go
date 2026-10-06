# dotman: Nix-style store

## Goals

1. Everything dotman builds lives in a store.
2. A store path's hash is computed from its inputs, before building.
3. If a store path exists, it is complete: that input has been built.
4. The same input always gives the same output and the same hash.

## Where we are

| Piece | Today | Nix-like? |
|---|---|---|
| Downloads | `PathForUrl(url, hash)`, temp dir + rename, `FakeHash`, `HashMismatchError` | Mostly: fixed-output, but keyed by URL too, and unpinned downloads are allowed |
| Package outputs | `CreatePath` → `uuid-name`, rebuilt every run | No |
| Profile | `BuildProfile` merges `bin/`, `share/`; atomic `SwitchProfile` | Like `buildEnv`, but uuid-named, no generations |
| Package code | `Install(log, cfg, store, storePath)` mixes evaluation and building | No |

## Core model: evaluation vs. realisation

A package doesn't build itself directly. It first **evaluates** to a
`Derivation`, which is plain, hashable data. A **builder** then realises that
derivation into a store path.

```go
// Pure data. Its canonical JSON (with inputs replaced by their output paths)
// is what the store hash is computed from.
type Derivation struct {
    Name    string
    System  string                 // "linux/amd64"
    Builder BuilderRef             // {Name: "yazi", Hash: <builder source hash>}
    Attrs   json.RawMessage        // the package's settings, already serialized
    Inputs  map[string]*Derivation // deps, fetches, local sources
}

type Package interface {
    // Derive must not do IO beyond hashing local files.
    Derive(ev *Eval) (*Derivation, error)
}

type BuildFunc func(b *Build) error

// b.Out            path to build into (a temp dir; see "Realisation")
// b.Input("zip")   realised store path of a named input
// b.Decode(&attrs) unmarshal Attrs into the builder's own struct
```

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
- `encoding/json` sorts map keys. Keep `Attrs` free of floats and other
  non-canonical values.
- **The store root is an input.** Outputs embed absolute paths, so the store
  dir is part of the hash, as in Nix.
- **System** (`GOOS/GOARCH`) is an input.

### Builder code is an input

If `writeWrappers` changes but the config doesn't, the hash has to change
anyway, or dotman silently keeps the old output. This is the main risk in the
whole design.

- Each builder package embeds its own source with
  `//go:embed *.go *.toml *.kdl` and `BuilderRef.Hash` is a hash of that FS.
- A global builder hash covers `lib/` sources, `go.sum` and
  `runtime.Version()`, and is mixed into every derivation.

### Global config is resolved during evaluation

`Install(cfg)` currently reads `cfg.Theme` while building. Instead, `Derive`
resolves the theme to its base16 colors and puts **the colors** into `Attrs`.
Editing a theme's colors then changes the hash, not only renaming the theme.

## Everything is a derivation

| Kind | Hash from | Notes |
|---|---|---|
| Fetch (fixed-output) | `name + content hash` only, not the URL | A mirror change doesn't refetch |
| Tree fetch | `name + hash of the extracted tree` | For GitHub source archives, which aren't byte-stable (like Nix's `fetchzip`) |
| Local source | file/dir content | `init.lua`, embedded plugins; copied into the store |
| Package | drv JSON | yazi, zellij, lazygit, pv, op, fzf |
| Profile | the package output paths | Merges `bin/`, `share/` |

- **Unpinned downloads are an error.** `FakeHash` stays as the way to discover
  the right hash.
- Tree hash: deterministic walk over sorted paths, hashing path + mode +
  content (or symlink target).

## Realisation

```
path = <store>/<hash>-<name>
if exists(path): done
tmp  = mkdtemp(store, ".tmp-")
build(tmp)
scanSelfRefs(tmp)     # fail if any file or symlink contains tmp
normalize(tmp)        # mtimes, modes, read-only
rename(tmp, path)     # lost a race and path exists: delete tmp, done
```

Inputs are realised first, in dependency order. This is the same pattern
`DownloadFile` already uses, applied to everything.

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

`scanSelfRefs` enforces this: if any file's contents or any symlink target
contains the temp path, the build fails with
`self-reference in bin/yazi; split the derivation`.

This keeps goal 3 exact (existence means complete) with no validity database.
As a bonus, changing a keybind rebuilds only `yazi-config` and the wrapper, and
the binary output stays as it is.

### Same input, same output

- Set mtimes to epoch + 1.
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
  a1b2…-yazi-x86_64-musl.zip/    fetch, path from content hash
  c4d5…-places.lua/              local source, path from content
  e6f7…-profile/
  .tmp-*/                        in-progress builds; safe to delete
~/.local/state/dotman/
  profile -> profiles/profile-12-link
  profiles/profile-12-link -> ../../../share/dotman/store/e6f7…-profile
```

- Each generation link records one profile, so rollback comes for free.
- Generation links are the GC roots.

## Garbage collection

- Mark everything reachable from the generation links. A path's references
  are found by scanning its files and symlinks for store hashes, as Nix does,
  so there's no references database.
- Sweep unreachable store paths and any leftover `.tmp-*` dirs.
- `dotman gc --keep N` deletes older generations first.

## Dependencies

- **Packaged deps** are `Inputs`. Wrappers use their absolute store paths.
- **Host deps** (e.g. `ffmpeg`, `7zz`) are declared as `HostDeps`, aren't
  hashed, and dotman warns about them. Don't pretend they're pure.

## CLI

| Command | Does |
|---|---|
| `dotman build [pkg…]` | Realise derivations without switching |
| `dotman switch` | Build the profile, add a generation, point `profile` at it |
| `dotman rollback` | Point `profile` at the previous generation |
| `dotman gc` | Delete unreachable store paths |
| `dotman show-drv <pkg>` | Print the hashed JSON, to answer "why did this rebuild?" |
| `dotman build --check` | Rebuild and compare, to verify determinism |

## Migration

1. **Core.** Add `Derivation`, `Eval`, `Build`, canonical hashing, builder
   source hashing and the realise loop (temp dir, self-ref scan, normalize,
   rename). Port `DownloadFile` to a fetch derivation keyed by content hash,
   and add `FetchTree`.
2. **First package.** Port `pv` end to end: local source input, builder hash,
   `HostDeps`.
3. **Profile.** Make the profile a derivation; add generations, `switch` and
   `rollback`.
4. **Real packages.** Port yazi (split into bin/config/wrapper; plugins as
   tree-fetch and local-source inputs; theme resolved in `Derive`), then
   zellij, lazygit, op and fzf.
5. **Hardening.** Read-only outputs, normalization, `build --check`, `gc`.
6. **Cleanup.** Remove `Install`, `CreatePath` and the uuid dependency.

## Open questions

1. **Host deps:** package heavy tools like ffmpeg, or keep them as declared
   `HostDeps`? Default: `HostDeps`.
2. **Sharing between machines:** if store paths should ever be copied between
   machines, the store root has to be fixed (like `/nix/store`) rather than
   under `$HOME`. Default: under `$HOME`, no sharing.
3. **Mutable state:** if any tool refuses to run with a read-only config dir,
   it needs an escape hatch (e.g. a wrapper that copies config to a writable
   location). Find out during step 4.
