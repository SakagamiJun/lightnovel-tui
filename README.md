<div align="center">
  <h1>LIGHTNOVELREADER (LNR)</h1>
  <p><strong>A Modern, High-Performance Terminal Light Novel Reader & Downloader</strong></p>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go" />
  <img alt="TUI" src="https://img.shields.io/badge/TUI-Bubble_Tea-00B4D8?style=flat-square" />
  <img alt="CLI" src="https://img.shields.io/badge/CLI-Cobra-4361EE?style=flat-square" />
  <img alt="License" src="https://img.shields.io/badge/License-MIT-blue?style=flat-square" />
  <img alt="Release" src="https://img.shields.io/github/v/release/SakagamiJun/lnovel_tui?style=flat-square&color=F05138" />
  <img alt="Platforms" src="https://img.shields.io/badge/Platforms-macOS%20%7C%20Linux%20%7C%20Windows-lightgray?style=flat-square" />
  <img alt="Size" src="https://img.shields.io/github/repo-size/SakagamiJun/lnovel_tui?style=flat-square" />
  <br /><br />
  <p>
    <a href="./README.zh-CN.md">中文</a> | <b>English</b>
  </p>
</div>

---

`lnr-core` is a lightweight, cross-platform terminal reader, downloader, and manager for light novel enthusiasts. Designed with Clean Architecture principles and built on Go's native streaming and concurrency primitives, `lnr` completely eschews heavy Electron/Webview stacks to deliver sub-millisecond responsiveness, minimal memory usage, immersive terminal typography, and standards-compliant EPUB generation.

---

## Core Features & Architecture

### 1. Dual-Track Presentation Layer
- **Command-Line Interface (`lnr`)**: Powered by Cobra, offering precise search, catalog extraction, multi-threaded streaming downloads, EPUB export, toplist exploration, and shell completion.
- **Terminal User Interface (`lnr-tui`)**: Powered by Bubble Tea & Lipgloss, responsive to dynamic terminal dimensions, featuring dark themes, intuitive keyboard navigation, and seamless viewport paging.

### 2. Streaming Pipelines & Zero Memory Overhead
- **Zero Memory Bloat**: Employs streaming transformations end-to-end (`sync.Pool` 32KB buffer pooling, streaming GB18030/UTF-8 transcoding, and streaming ZIP/EPUB writers).
- **Process-Level Efficiency**: CLI and TUI share domain models directly through native Go APIs without subprocess overhead or stdout scraping.
- **Clean Visual Presentation**: Professional textual badges (e.g., `[Bookshelf]`, `[Search]`, `[Explore]`, `[Settings]`), strictly avoiding visual clutter.

### 3. Immersive Terminal Reading
- **Typography & Alignment**: Automatic East Asian character width alignment, smart word wrapping, and text sanitization rules (ellipsis/dash normalization and ad filter).
- **Simplified / Traditional Chinese Toggle**: Seamless instant text conversion at the press of `t`.
- **Automatic Bookmarks**: Tracks reading position by line number; instantly resumes where you left off.
- **Themes**: Switch between multiple color palettes (Noir, Warm Sepia, Forest Pine).

### 4. Native Terminal Illustration Viewer
- **Protocol Adaptation**: Supports Kitty graphics protocol, iTerm2 inline images, and Sixel for high-definition illustrations directly in compatible terminals.
- **System QuickLook**: Triggers macOS native QuickLook window for instant inspection.

### 5. Standards-Compliant EPUB Generation
- **Granular Export**: Export entire novel or split by volume into separate EPUB files.
- **Text-Only Option**: Separate full illustrated edition from ultra-compact pure text edition.
- **Reader Compatibility**: Compliant with EPUB 3.0 standards, compatible with Apple Books, Calibre, Kindle, and E-ink readers.

### 6. Bookshelf & Storage Management
- **Smart Bookshelf**: Multi-group organization, pinned books, and multi-criteria dynamic sorting (recent read, word count, completion status).
- **Update Tracking**: One-key online check to discover newly published volumes and chapters.
- **Deep Storage Analysis**: Visual breakdown of text, images, and exported EPUBs with safe illustration cleanup (releasing 90%+ space while retaining covers and text).

---

## Installation

### macOS One-Line Install (Homebrew)

```bash
brew tap SakagamiJun/tap
brew trust SakagamiJun/tap
brew install lnr
```
*(Note: Per recent Homebrew security updates, run `brew trust SakagamiJun/tap` before installing from a third-party tap)*

Both `lnr` (CLI) and `lnr-tui` (TUI) will be installed into your system path.

### Pre-Built Binaries (GitHub Releases)

Download pre-compiled binaries from the [Releases page](https://github.com/SakagamiJun/lnovel_tui/releases):
- macOS (Apple Silicon & Intel)
- Linux (x86_64 & ARM64)
- Windows (x86_64)

### Go Install

Requires Go 1.22+:
```bash
go install lnr-core/cmd/lnr@latest
go install lnr-core/cmd/lnr-tui@latest
```

### Build from Source

```bash
git clone https://github.com/SakagamiJun/lnovel_tui.git
cd lnovel_tui
make build
```
Binaries will be output to `bin/lnr` and `bin/lnr-tui`.

---

## Quickstart

### 1. Terminal Reader (TUI)

Launch the interactive reader:
```bash
lnr-tui
```

#### Keymap Navigation

| Key | Action | Description |
| :--- | :--- | :--- |
| `Tab` / `Shift+Tab` | Switch Tab | Cycle between Bookshelf, Explore, Search, Settings |
| `↑` / `↓` or `k` / `j` | Move Cursor | Navigate books, chapters, or settings |
| `Enter` | Select / Open | Open novel catalog, read chapter, or trigger option |
| `Esc` | Back / Cancel | Return to previous view, exit modal, or cancel |
| `q` | Quit | Exit program cleanly from main screens |
| `Ctrl+C` | Force Exit | Immediately exit program from any view |

#### Reading View Shortcuts

| Key | Action | Description |
| :--- | :--- | :--- |
| `j` / `k` or `↓` / `↑` | Scroll Line | Scroll text viewport by configured step |
| `Space` / `PageDown` | Page Down | Scroll down by a full viewport screen |
| `b` / `PageUp` | Page Up | Scroll up by a full viewport screen |
| `[` / `]` | Chapter Jump | Jump to previous or next chapter |
| `t` | Toggle S/T | Toggle Simplified and Traditional Chinese |
| `c` | Cycle Theme | Switch between reader color schemes |
| `i` | View Images | Open illustration modal and inline viewer |
| `Esc` | Exit Reader | Saves bookmark and returns to catalog |

---

### 2. Command-Line Interface (CLI)

```bash
# 1. Search for novels by title or author
lnr search "关于我转生变成史莱姆这档事"
lnr search "伏濑" -a

# 2. View details and volume catalog
lnr info 4340

# 3. Explore Wenku8 toplists
lnr top anime
lnr top allvisit

# 4. Download chapters and illustrations
lnr download 4340               # Full book
lnr download 4340 --volume 1    # Volume 1 only

# 5. Export to EPUB
lnr export 4340 -o ./slime_complete.epub
lnr export 4340 --volume 1 -o ./slime_vol1.epub
lnr export 4340 --split-volume -o ./dist/        # Batch split by volume
lnr export 4340 --no-images -o ./slime_text.epub # Text-only

# 6. Check bookshelf online updates
lnr update

# 7. Check version and build details
lnr version
lnr --version

# 8. Generate shell completion
lnr completion zsh > ~/.zfunc/_lnr
```

---

## Testing & Quality Assurance

Run all test suites with data race detection:
```bash
make test
```

---

## Acknowledgments

- [Charmbracelet](https://charm.sh/) - For the exceptional modern terminal libraries (`bubbletea`, `lipgloss`, `bubbles`).
- [Wenku8](https://www.wenku8.net/) - For metadata and light novel resources.
- [Cobra](https://github.com/spf13/cobra) - For robust CLI command infrastructure.

---

## License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.
