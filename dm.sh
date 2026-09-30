GO_VERSION=1.27.1
TMP_DIR=.tmp
GO_TARBALL="go${GO_VERSION}.linux-amd64.tar.gz"

log() {
    echo "[$1] $2"
}

install_go_tmp() {
    log "INFO" "Installing Go $GO_VERSION in $TMP_DIR..."
    mkdir -p "$TMP_DIR" || return 1

    curl -fL --output-dir "$TMP_DIR" -O "https://go.dev/dl/$GO_TARBALL" || {
        echo "Failed to download Go $GO_VERSION" >&2
        return 1
    }

    tar -C "$TMP_DIR" -xzf "$TMP_DIR/$GO_TARBALL" || {
        echo "Failed to extract Go" >&2
        return 1
    }

    rm -f "$TMP_DIR/$GO_TARBALL"
    log "INFO" "Go $(go version) installed successfully."
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
    exec_go run dotman/main.go "$@"
}

ensure_go_installed
exec_dotman "$@"
