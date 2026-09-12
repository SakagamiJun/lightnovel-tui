#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TARGET_OS="${TARGET_OS:-$(go env GOOS)}"
TARGET_ARCH="${TARGET_ARCH:-$(go env GOARCH)}"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}"
BUILD_DATE="${BUILD_DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
OUTPUT_DIR="${OUTPUT_DIR:-bin}"

EXT=""
if [[ "$TARGET_OS" == "windows" ]]; then
  EXT=".exe"
fi

mkdir -p "$OUTPUT_DIR"

LDFLAGS="-s -w \
  -X github.com/SakagamiJun/lightnovel-tui/pkg/version.Version=${VERSION} \
  -X github.com/SakagamiJun/lightnovel-tui/pkg/version.GitCommit=${GIT_COMMIT} \
  -X github.com/SakagamiJun/lightnovel-tui/pkg/version.BuildDate=${BUILD_DATE}"

echo "==> Building for ${TARGET_OS}/${TARGET_ARCH} (version: ${VERSION}, commit: ${GIT_COMMIT})..."

CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" \
  go build -trimpath -ldflags "$LDFLAGS" -o "${OUTPUT_DIR}/lnr${EXT}" ./cmd/lnr

CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" \
  go build -trimpath -ldflags "$LDFLAGS" -o "${OUTPUT_DIR}/lnr-tui${EXT}" ./cmd/lnr-tui

echo "==> Successfully built ${OUTPUT_DIR}/lnr${EXT} and ${OUTPUT_DIR}/lnr-tui${EXT}"
