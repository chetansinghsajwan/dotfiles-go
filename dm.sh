#!/usr/bin/env bash
set -euo pipefail

GO_VERSION=1.27.1
TMP_DIR=.tmp

detect_go_os() {
    case "$(uname -s)" in
        Linux) echo "linux" ;;
        Darwin) echo "darwin" ;;
        *)
            echo "Unsupported OS: $(uname -s)" >&2
            return 1
            ;;
    esac
}

detect_go_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *)
            echo "Unsupported architecture: $(uname -m)" >&2
            return 1
            ;;
    esac
}

log() {
    echo "[$1] $2"
}

install_go_tmp() {
    local go_os go_arch go_tarball max_attempts attempt
    go_os="$(detect_go_os)" || return 1
    go_arch="$(detect_go_arch)" || return 1
    go_tarball="go${GO_VERSION}.${go_os}-${go_arch}.tar.gz"
    max_attempts=3

    mkdir -p "$TMP_DIR" || return 1

    for attempt in $(seq 1 "$max_attempts"); do
        log "INFO" "Installing Go $GO_VERSION ($go_os/$go_arch) in $TMP_DIR... (attempt $attempt/$max_attempts)"
        rm -f "$TMP_DIR/$go_tarball"

        if ! curl -fL --output-dir "$TMP_DIR" -O "https://go.dev/dl/$go_tarball"; then
            echo "Failed to download Go $GO_VERSION (attempt $attempt/$max_attempts)" >&2
            continue
        fi

        # The tarball can arrive complete-but-corrupt (e.g. flaky network
        # mangling bytes in transit) without curl noticing, so verify it
        # before trusting it to tar.
        if ! gzip -t "$TMP_DIR/$go_tarball" 2>/dev/null; then
            echo "Downloaded archive is corrupt (attempt $attempt/$max_attempts)" >&2
            continue
        fi

        rm -rf "$TMP_DIR/go"
        if ! tar -C "$TMP_DIR" -xzf "$TMP_DIR/$go_tarball"; then
            echo "Failed to extract Go (attempt $attempt/$max_attempts)" >&2
            continue
        fi

        rm -f "$TMP_DIR/$go_tarball"
        log "INFO" "Go installed successfully: $("$TMP_DIR/go/bin/go" version)."
        return 0
    done

    echo "Failed to install Go $GO_VERSION after $max_attempts attempts" >&2
    return 1
}

ensure_go_installed() {
    if [ ! -x "$TMP_DIR/go/bin/go" ]; then
        install_go_tmp
    fi
}

exec_go() {
    "$TMP_DIR/go/bin/go" "$@"
}

exec_dotman() {
    exec_go run . "$@"
}

ensure_go_installed
exec_dotman "$@"
