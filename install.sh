#!/bin/sh
# Helm installer — downloads a verified release artifact and installs the
# canonical `helm` executable plus its aliases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/divijg19/Helm/main/install.sh | sh
#   VERSION=vX.Y.Z sh install.sh   # pin a release tag; omit to install the latest
#   INSTALL_DIR=/usr/local/bin sh install.sh
#
# VERSION must name a published release tag: the download URL is built from it
# directly, so an unpublished or mistyped tag fails with a download error.
#
# Downloaded content is never executed before verification: every artifact is
# checked against the published SHA-256 checksums before extraction, and only
# then does the installer run the extracted binary once for a version check.
# Portable POSIX sh only; no bashisms. Requires mktemp(1) and install(1),
# which are not part of POSIX and may be missing from very minimal images.

set -eu

REPO="divijg19/Helm"   # matches the module path; GitHub paths are case-insensitive
BINARY="helm"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-}"

cleanup() {
    if [ -n "${WORKDIR:-}" ] && [ -d "${WORKDIR}" ]; then
        rm -rf "$WORKDIR"
    fi
}

die() {
    echo "install.sh: error: $*" >&2
    exit 1
}

# --- platform detection -----------------------------------------------------

detect_os() {
    os="$(uname -s)"
    case "$os" in
        Linux) echo "linux" ;;
        Darwin) echo "darwin" ;;
        *) die "unsupported platform: $os (only Linux and macOS are supported by this installer)" ;;
    esac
}

detect_arch() {
    arch="$(uname -m)"
    case "$arch" in
        x86_64 | amd64) echo "amd64" ;;
        aarch64 | arm64) echo "arm64" ;;
        *) die "unsupported architecture: $arch" ;;
    esac
}

OS="$(detect_os)"
ARCH="$(detect_arch)"

# --- version resolution ------------------------------------------------------

resolve_version() {
    if [ -n "$VERSION" ]; then
        echo "$VERSION"
        return
    fi
    echo "Resolving latest release..." >&2
    tag="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest" \
        | sed 's#.*/tag/##')"
    [ -n "$tag" ] || die "could not determine latest release version"
    echo "$tag"
}

VERSION="$(resolve_version)"

# --- checksum helper ---------------------------------------------------------

sha256_of() {
    f="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$f" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$f" | awk '{print $1}'
    else
        die "no SHA-256 tool (sha256sum/shasum) available"
    fi
}

# --- download + verify -------------------------------------------------------

WORKDIR="$(mktemp -d)" || die "cannot create temporary directory"
# Arm the cleanup trap only now that WORKDIR holds a directory this script
# created. It must never be armed earlier: TMPDIR is a standard variable that
# macOS and many CI images export, so a trap registered before this assignment
# would delete the caller's pre-existing temporary directory on any early exit.
trap cleanup EXIT INT TERM
cd "$WORKDIR" || die "cannot enter temporary directory"

ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

echo "Downloading Helm ${VERSION} (${OS}/${ARCH})..." >&2

curl -fsSL "${BASE_URL}/${ARCHIVE}" -o "$ARCHIVE" \
    || die "download failed: ${ARCHIVE}"
curl -fsSL "${BASE_URL}/checksums.txt" -o checksums.txt \
    || die "download failed: checksums.txt"

EXPECTED="$(grep " ${ARCHIVE}\$" checksums.txt | awk '{print $1}')"
[ -n "$EXPECTED" ] || die "checksum entry not found for ${ARCHIVE}"
ACTUAL="$(sha256_of "$ARCHIVE")"

if [ "$EXPECTED" != "$ACTUAL" ]; then
    die "checksum mismatch for ${ARCHIVE} (expected ${EXPECTED}, got ${ACTUAL})"
fi

echo "Verifying checksum..." >&2

# --- extract + install -------------------------------------------------------

tar -xzf "$ARCHIVE" \
    || die "failed to extract ${ARCHIVE}"
[ -f "$BINARY" ] || die "archive did not contain ${BINARY}"

mkdir -p "$INSTALL_DIR" \
    || die "cannot create install directory: ${INSTALL_DIR}"

if [ ! -w "$INSTALL_DIR" ]; then
    die "no write permission for ${INSTALL_DIR} (set INSTALL_DIR to a writable path)"
fi

TARGET="${INSTALL_DIR}/${BINARY}"
install -m 0755 "$BINARY" "$TARGET.tmp" \
    || die "failed to stage ${BINARY}"
mv "$TARGET.tmp" "$TARGET" \
    || die "failed to install ${BINARY} to ${INSTALL_DIR}"

echo "Installing to ${INSTALL_DIR}..." >&2

# Aliases resolve through the single canonical binary by basename.
for alias in Helm update-go-tools; do
    ln -sf "$TARGET" "${INSTALL_DIR}/${alias}"
done

# --- verify + report --------------------------------------------------------

if ! "$TARGET" --version >/dev/null 2>&1; then
    die "installed binary failed to report its version"
fi

case ":$PATH:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
        echo "Note: ${INSTALL_DIR} is not on your PATH. Add it with:" >&2
        echo "    export PATH=\"${INSTALL_DIR}:\$PATH\"" >&2
        ;;
esac

echo "Helm ${VERSION} installed successfully to ${INSTALL_DIR}." >&2
