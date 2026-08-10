#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
CLEAN_VERSION="${VERSION#v}"
DIST_DIR="${DIST_DIR:-dist}"
TARGET_OS="${TARGET_OS:-$(go env GOOS)}"
TARGET_ARCH="${TARGET_ARCH:-$(go env GOARCH)}"

mkdir -p "$DIST_DIR"

STAGE_DIR="$(mktemp -d /tmp/lnr-package-XXXXXX)"
trap 'rm -rf "$STAGE_DIR"' EXIT

echo "==> Packaging ${TARGET_OS}-${TARGET_ARCH} for version ${VERSION}..."

TARGET_OS="$TARGET_OS" TARGET_ARCH="$TARGET_ARCH" VERSION="$VERSION" OUTPUT_DIR="$STAGE_DIR" ./scripts/build.sh

cp README.md LICENSE "$STAGE_DIR/"

ARCHIVE_NAME="lnr-v${CLEAN_VERSION}-${TARGET_OS}-${TARGET_ARCH}"

if [[ "$TARGET_OS" == "windows" ]]; then
  ARCHIVE_FILE="${ARCHIVE_NAME}.zip"
  (cd "$STAGE_DIR" && zip -q -r "${ROOT}/${DIST_DIR}/${ARCHIVE_FILE}" .)
else
  ARCHIVE_FILE="${ARCHIVE_NAME}.tar.gz"
  tar -czf "${DIST_DIR}/${ARCHIVE_FILE}" -C "$STAGE_DIR" .
fi

echo "==> Created ${DIST_DIR}/${ARCHIVE_FILE}"

# Generate / update checksums.txt
cd "$DIST_DIR"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$ARCHIVE_FILE" >> checksums.txt.tmp
elif command -v shasum >/dev/null 2>&1; then
  shasum -a 256 "$ARCHIVE_FILE" >> checksums.txt.tmp
fi

if [[ -f checksums.txt.tmp ]]; then
  # Deduplicate entries by file name
  awk '!seen[$2]++' checksums.txt.tmp > checksums.txt
  rm -f checksums.txt.tmp
  echo "==> Updated ${DIST_DIR}/checksums.txt"
fi
