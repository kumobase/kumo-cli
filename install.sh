#!/bin/sh
# kumo — Kumo platform CLI installer
#
# Usage (one-liner):
#   curl -sSfL https://raw.githubusercontent.com/kumobase/kumo-cli/main/install.sh | sh
#
# Installer-time environment variables (distinct from the runtime CLI's
# KUMO_API_KEY / KUMO_BASE_URL etc., which this script does not touch):
#   KUMO_VERSION       Version to install (e.g. "v1.2.3"). Default: latest release.
#   KUMO_INSTALL_DIR   Where to put the binary. Default: ~/.local/bin then
#                      /usr/local/bin if the former isn't writable.
#   KUMO_OS, KUMO_ARCH Override OS / arch detection.
#
# What this script does:
#   1. Detects OS + architecture (or honours KUMO_OS / KUMO_ARCH).
#   2. Resolves the version to install (latest release, or KUMO_VERSION).
#   3. Downloads the matching archive + checksums file from GitHub releases.
#   4. Verifies the SHA-256 checksum.
#   5. Extracts the binary into KUMO_INSTALL_DIR.
#   6. chmod +x and prints the version it installed.
#
# POSIX-portable — works under /bin/sh on Linux + macOS. No bashisms.

set -eu

GH_REPO="kumobase/kumo-cli"
BINARY="kumo"

# Output helpers ----------------------------------------------------------

# All info goes to stderr so the script can be piped without
# interleaving installer prose into the binary.
info()  { printf '==> %s\n' "$*" >&2; }
warn()  { printf 'WARN: %s\n' "$*" >&2; }
fatal() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

# Dependencies ------------------------------------------------------------

have() { command -v "$1" >/dev/null 2>&1; }

if   have curl; then DL="curl -sSfL -o"
elif have wget; then DL="wget -qO"
else fatal "need curl or wget on PATH"
fi

# tar required for tar.gz archives (we don't ship Windows zip via this script).
have tar  || fatal "tar not found"
have uname || fatal "uname not found"

# Detect OS ---------------------------------------------------------------

detect_os() {
  if [ -n "${KUMO_OS:-}" ]; then echo "$KUMO_OS"; return; fi
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    linux)  echo "linux" ;;
    darwin) echo "macos" ;;  # goreleaser name_template maps darwin → macos
    *)      fatal "unsupported OS: $os (this script supports linux + macos; for Windows download the .zip from https://github.com/$GH_REPO/releases)" ;;
  esac
}

# Detect architecture -----------------------------------------------------

detect_arch() {
  if [ -n "${KUMO_ARCH:-}" ]; then echo "$KUMO_ARCH"; return; fi
  arch=$(uname -m)
  case "$arch" in
    x86_64|amd64) echo "x86_64" ;;  # goreleaser maps amd64 → x86_64
    arm64|aarch64) echo "arm64" ;;
    *) fatal "unsupported arch: $arch (need x86_64 or arm64)" ;;
  esac
}

# Resolve version ---------------------------------------------------------

# latest_tag_via_redirect prints the latest release tag by following the
# github.com "releases/latest" redirect (→ /releases/tag/vX.Y.Z). It uses the
# github.com host, which — unlike api.github.com — is NOT subject to the
# 60-request/hour anonymous rate limit that 403s on shared / CI / container IPs.
# Prints nothing if the tag can't be determined (caller falls back to the API).
latest_tag_via_redirect() {
  url="https://github.com/$GH_REPO/releases/latest"
  eff=""
  if have curl; then
    eff=$(curl -sSL -o /dev/null -w '%{url_effective}' "$url" 2>/dev/null) || eff=""
  elif have wget; then
    # Stop at the first redirect and read the Location header (GNU wget). On
    # wget builds without -S/--max-redirect this yields nothing and we fall back.
    eff=$(wget -q -S --max-redirect=0 -O /dev/null "$url" 2>&1 \
            | sed -n 's/^[[:space:]]*[Ll]ocation:[[:space:]]*//p' | head -n1 | tr -d '\r')
  fi
  case "$eff" in
    */releases/tag/*) printf '%s\n' "${eff##*/tag/}" ;;
    *) : ;;  # no releases yet / unexpected shape → print nothing
  esac
}

# latest_tag_via_api prints the latest tag via the GitHub API. Fallback only —
# it is subject to the anonymous rate limit and may 403.
latest_tag_via_api() {
  api="https://api.github.com/repos/$GH_REPO/releases/latest"
  tmp=$(mktemp)
  if $DL "$tmp" "$api" 2>/dev/null; then
    grep -m1 '"tag_name":' "$tmp" | sed -e 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/'
  fi
  rm -f "$tmp"
}

resolve_version() {
  if [ -n "${KUMO_VERSION:-}" ]; then
    echo "$KUMO_VERSION"; return
  fi
  info "looking up latest release of $GH_REPO"
  ver=$(latest_tag_via_redirect)
  [ -n "$ver" ] || ver=$(latest_tag_via_api)
  [ -n "$ver" ] || fatal "could not determine the latest release (GitHub may be rate-limiting this IP). Pin a version instead: curl -sSfL .../install.sh | KUMO_VERSION=vX.Y.Z sh"
  echo "$ver"
}

# Resolve install directory ----------------------------------------------

resolve_install_dir() {
  if [ -n "${KUMO_INSTALL_DIR:-}" ]; then
    mkdir -p "$KUMO_INSTALL_DIR" 2>/dev/null || fatal "cannot create $KUMO_INSTALL_DIR"
    echo "$KUMO_INSTALL_DIR"; return
  fi
  if mkdir -p "$HOME/.local/bin" 2>/dev/null && [ -w "$HOME/.local/bin" ]; then
    echo "$HOME/.local/bin"; return
  fi
  if [ -w /usr/local/bin ]; then
    echo "/usr/local/bin"; return
  fi
  fatal "neither \$HOME/.local/bin nor /usr/local/bin is writable; set KUMO_INSTALL_DIR=/path/to/bin and re-run"
}

# Main --------------------------------------------------------------------

OS=$(detect_os)
ARCH=$(detect_arch)
VERSION=$(resolve_version)
INSTALL_DIR=$(resolve_install_dir)

# goreleaser strips the leading "v" from the version in archive names.
ver_no_v=$(printf '%s' "$VERSION" | sed -e 's/^v//')
archive="${BINARY}_${ver_no_v}_${OS}_${ARCH}.tar.gz"
base_url="https://github.com/$GH_REPO/releases/download/$VERSION"

info "version:     $VERSION"
info "os/arch:     $OS/$ARCH"
info "archive:     $archive"
info "destination: $INSTALL_DIR/$BINARY"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

info "downloading archive"
$DL "$tmpdir/$archive" "$base_url/$archive" || fatal "failed to download $base_url/$archive"

info "downloading checksums"
$DL "$tmpdir/checksums.txt" "$base_url/checksums.txt" || fatal "failed to download checksums.txt"

# Verify checksum ---------------------------------------------------------

if   have sha256sum; then
  expected=$(grep "  $archive\$" "$tmpdir/checksums.txt" | awk '{print $1}')
  actual=$(sha256sum "$tmpdir/$archive" | awk '{print $1}')
elif have shasum; then
  expected=$(grep "  $archive\$" "$tmpdir/checksums.txt" | awk '{print $1}')
  actual=$(shasum -a 256 "$tmpdir/$archive" | awk '{print $1}')
else
  warn "no sha256sum / shasum available — skipping checksum verification (not recommended)"
  expected=""; actual=""
fi
if [ -n "$expected" ]; then
  [ "$expected" = "$actual" ] || fatal "checksum mismatch for $archive: expected $expected, got $actual"
  info "checksum OK"
fi

# Extract + install -------------------------------------------------------

info "extracting"
tar -xzf "$tmpdir/$archive" -C "$tmpdir" || fatal "extract failed"
[ -f "$tmpdir/$BINARY" ] || fatal "expected $BINARY in archive but it's missing"

install -m 0755 "$tmpdir/$BINARY" "$INSTALL_DIR/$BINARY" 2>/dev/null \
  || cp "$tmpdir/$BINARY" "$INSTALL_DIR/$BINARY" \
  || fatal "could not write to $INSTALL_DIR/$BINARY"
chmod 0755 "$INSTALL_DIR/$BINARY"

# Verify the binary runs --------------------------------------------------

info "installed:   $INSTALL_DIR/$BINARY"
"$INSTALL_DIR/$BINARY" --version 2>&1 || warn "binary installed but 'kumo --version' failed — try running it manually"

# PATH hint ---------------------------------------------------------------

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) info "NOTE: $INSTALL_DIR is not in your \$PATH yet. Add this to your shell profile:"
     info "      export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac

info "done. try: kumo auth login"
