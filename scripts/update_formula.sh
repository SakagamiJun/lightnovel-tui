#!/usr/bin/env bash
set -euo pipefail

TAP_DIR="${TAP_DIR:?TAP_DIR must be set}"
VERSION="${VERSION:?VERSION must be set}"
CLEAN_VERSION="${VERSION#v}"

DARWIN_ARM64_SHA="${DARWIN_ARM64_SHA:-0000000000000000000000000000000000000000000000000000000000000000}"
DARWIN_AMD64_SHA="${DARWIN_AMD64_SHA:-0000000000000000000000000000000000000000000000000000000000000000}"
LINUX_ARM64_SHA="${LINUX_ARM64_SHA:-0000000000000000000000000000000000000000000000000000000000000000}"
LINUX_AMD64_SHA="${LINUX_AMD64_SHA:-0000000000000000000000000000000000000000000000000000000000000000}"

FORMULA_DIR="${TAP_DIR}/Formula"
mkdir -p "$FORMULA_DIR"
FORMULA_FILE="${FORMULA_DIR}/lnr.rb"

cat <<EOF > "$FORMULA_FILE"
class Lnr < Formula
  desc "Modern, high-performance CLI & TUI light novel reader and downloader"
  homepage "https://github.com/SakagamiJun/lnovel_tui"
  version "${CLEAN_VERSION}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/SakagamiJun/lnovel_tui/releases/download/v#{version}/lnr-v#{version}-darwin-arm64.tar.gz"
      sha256 "${DARWIN_ARM64_SHA}"
    else
      url "https://github.com/SakagamiJun/lnovel_tui/releases/download/v#{version}/lnr-v#{version}-darwin-amd64.tar.gz"
      sha256 "${DARWIN_AMD64_SHA}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/SakagamiJun/lnovel_tui/releases/download/v#{version}/lnr-v#{version}-linux-arm64.tar.gz"
      sha256 "${LINUX_ARM64_SHA}"
    else
      url "https://github.com/SakagamiJun/lnovel_tui/releases/download/v#{version}/lnr-v#{version}-linux-amd64.tar.gz"
      sha256 "${LINUX_AMD64_SHA}"
    end
  end

  def install
    bin.install "lnr"
    bin.install "lnr-tui"
    bin.install_symlink "lnr-tui" => "lnovel-tui"

    generate_completions_from_executable(bin/"lnr", "completion")
  end

  test do
    assert_match "LightNovelReader CLI", shell_output("#{bin}/lnr --help")
    assert_match "version", shell_output("#{bin}/lnr version")
  end
end
EOF

echo "==> Successfully wrote Homebrew formula to ${FORMULA_FILE}"
