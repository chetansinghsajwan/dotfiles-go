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
    local go_os go_arch go_tarball
    go_os="$(detect_go_os)" || return 1
    go_arch="$(detect_go_arch)" || return 1
    go_tarball="go${GO_VERSION}.${go_os}-${go_arch}.tar.gz"

    log "INFO" "Installing Go $GO_VERSION ($go_os/$go_arch) in $TMP_DIR..."
    mkdir -p "$TMP_DIR" || return 1

    curl -fL --output-dir "$TMP_DIR" -O "https://go.dev/dl/$go_tarball" || {
        echo "Failed to download Go $GO_VERSION" >&2
        return 1
    }

    rm -rf "$TMP_DIR/go"
    tar -C "$TMP_DIR" -xzf "$TMP_DIR/$go_tarball" || {
        echo "Failed to extract Go" >&2
        return 1
    }

    rm -f "$TMP_DIR/$go_tarball"
    log "INFO" "Go installed successfully: $("$TMP_DIR/go/bin/go" version)."
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
