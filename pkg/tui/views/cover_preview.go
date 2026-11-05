package views

import (
	"context"
	"errors"
	"fmt"
	stdimage "image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SakagamiJun/lightnovel-tui/pkg/downloader"
	termimage "github.com/SakagamiJun/lightnovel-tui/pkg/image"
	"github.com/SakagamiJun/lightnovel-tui/pkg/source"
	"github.com/SakagamiJun/lightnovel-tui/pkg/storage"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/theme"
)

var (
	errCoverUnavailable = errors.New("当前终端不支持封面内联渲染")
	errCoverEmpty       = errors.New("没有可用的封面地址或本地缓存")
)

// CoverOwner identifies which view initiated a cover load.
type CoverOwner int

const (
	CoverOwnerNone CoverOwner = iota
	CoverOwnerBookshelf
	CoverOwnerCatalog
)

// coverPreview is a reusable single-image preview backed by Kitty Unicode
// placeholders. The owning view decides the pane geometry and when a load is
// eligible; this type owns download, transmission, resize and cleanup state.
type coverPreview struct {
	out     *termimage.LockedWriter
	capable bool
	store   *storage.Storage
	src     source.DataSource
	bookID  string
	owner   CoverOwner

	url     string
	path    string
	loading bool
	err     error

	imageID     uint32
	grid        []string
	cols        int
	rows        int
	imageWidth  int
	imageHeight int

	gen    atomic.Uint64
	layout atomic.Uint64
	mu     sync.Mutex
}

// CoverLoadedMsg carries a cover download/transmission result to the owning
// view. AppModel routes it independently of the currently active tab.
type CoverLoadedMsg struct {
	owner   CoverOwner
	gen     uint64
	path    string
	imageID uint32
	cols    int
	rows    int
	width   int
	height  int
	kitty   bool
	stale   bool
	err     error
}

// SetTerminalOutput enables the preview when the terminal supports Kitty
// Unicode placeholders.
func (c *coverPreview) SetTerminalOutput(out *termimage.LockedWriter) {
	c.out = out
	c.capable = out != nil && termimage.SupportsKittyPlaceholders()
}

// Configure sets the image source metadata without changing the currently
// displayed image. Call Reset first when switching books.
func (c *coverPreview) Configure(store *storage.Storage, src source.DataSource, bookID, url, path string, owner CoverOwner) {
	c.store = store
	c.src = src
	c.bookID = bookID
	c.owner = owner
	c.url = url
	c.path = path
}

// Owner returns the view that initiated the load.
func (m CoverLoadedMsg) Owner() CoverOwner {
	return m.owner
}

// HasCandidate reports whether the current book has a local cover or a remote
// cover URL to try.
func (c *coverPreview) HasCandidate() bool {
	return c.url != "" || c.path != ""
}

// HasImage reports whether an image grid is currently installed.
func (c *coverPreview) HasImage() bool {
	return c.imageID != 0 && len(c.grid) > 0
}

// Reset invalidates in-flight work, clears the preview and schedules deletion
// of the terminal-side image.
func (c *coverPreview) Reset() tea.Cmd {
	c.gen.Add(1)
	c.url = ""
	c.path = ""
	c.loading = false
	c.err = nil
	c.grid = nil
	c.cols = 0
	c.rows = 0
	c.imageWidth = 0
	c.imageHeight = 0
	id := c.imageID
	c.imageID = 0
	return c.DeleteCmd(id)
}

// ClearForReupload resets the displayed image identity after the terminal has
// cleared all Kitty data (for example on resume) while keeping the source
// metadata for a fresh load.
func (c *coverPreview) ClearForReupload() {
	c.gen.Add(1)
	c.loading = false
	c.grid = nil
	c.cols = 0
	c.rows = 0
	c.imageID = 0
}

// StartLoadIfNeeded starts a load when a candidate exists and no image is
// currently loading or displayed.
func (c *coverPreview) StartLoadIfNeeded(maxCols, maxRows int) tea.Cmd {
	if !c.capable || c.out == nil || c.store == nil || maxCols <= 0 || maxRows <= 0 {
		return nil
	}
	if !c.HasCandidate() || c.loading || c.imageID != 0 || len(c.grid) > 0 {
		return nil
	}
	return c.LoadCmd(maxCols, maxRows)
}

// LoadCmd downloads the cover when only a URL is known, decodes its dimensions
// and streams it to the terminal. The command is generation-guarded so a book
// switch or resize can discard late results.
func (c *coverPreview) LoadCmd(maxCols, maxRows int) tea.Cmd {
	c.loading = true
	c.err = nil
	c.grid = nil
	c.cols = 0
	c.rows = 0

	gen := c.gen.Add(1)
	owner := c.owner
	previousID := c.imageID
	bookID := c.bookID
	coverURL := c.url
	localPath := c.path
	out := c.out
	store := c.store
	src := c.src
	capable := c.capable

	return func() tea.Msg {
		if c.gen.Load() != gen {
			return CoverLoadedMsg{owner: owner, gen: gen, stale: true}
		}
		if store == nil {
			return CoverLoadedMsg{owner: owner, gen: gen, err: errCoverUnavailable}
		}

		if localPath != "" {
			if _, err := os.Stat(localPath); err != nil {
				localPath = ""
			}
		}
		if localPath == "" {
			if coverURL == "" {
				return CoverLoadedMsg{owner: owner, gen: gen, err: errCoverEmpty}
			}
			dl, err := downloader.NewDownloader(src, store)
			if err != nil {
				return CoverLoadedMsg{owner: owner, gen: gen, err: fmt.Errorf("初始化封面下载器失败: %w", err)}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			path, err := dl.DownloadCover(ctx, bookID, coverURL)
			cancel()
			if err != nil {
				return CoverLoadedMsg{owner: owner, gen: gen, err: fmt.Errorf("下载封面失败: %w", err)}
			}
			localPath = path
		}

		if localPath == "" {
			return CoverLoadedMsg{owner: owner, gen: gen, err: errCoverEmpty}
		}
		if c.gen.Load() != gen {
			return CoverLoadedMsg{owner: owner, gen: gen, stale: true}
		}
		if !capable || out == nil {
			return CoverLoadedMsg{owner: owner, gen: gen, err: errCoverUnavailable}
		}

		f, err := os.Open(localPath)
		if err != nil {
			return CoverLoadedMsg{owner: owner, gen: gen, err: err}
		}
		cfg, _, decodeErr := stdimage.DecodeConfig(f)
		_ = f.Close()
		if decodeErr != nil {
			return CoverLoadedMsg{owner: owner, gen: gen, err: fmt.Errorf("解析封面尺寸失败: %w", decodeErr)}
		}
		imgW, imgH := cfg.Width, cfg.Height
		if imgW <= 0 || imgH <= 0 {
			return CoverLoadedMsg{owner: owner, gen: gen, err: errCoverEmpty}
		}

		cols, rows := termimage.CalculateScaledDimensions(imgW, imgH, maxCols, maxRows)
		newID := termimage.NewKittyImageID()

		c.mu.Lock()
		defer c.mu.Unlock()
		if c.gen.Load() != gen {
			return CoverLoadedMsg{owner: owner, gen: gen, stale: true}
		}

		transmitErr := out.WithLock(func(w io.Writer) error {
			if err := termimage.TransmitKittyImage(w, localPath, newID, cols, rows); err != nil {
				_ = termimage.DeleteKittyImage(w, newID)
				return err
			}
			if previousID != 0 && previousID != newID {
				_ = termimage.DeleteKittyImage(w, previousID)
			}
			return nil
		})
		if transmitErr != nil {
			return CoverLoadedMsg{owner: owner, gen: gen, err: transmitErr}
		}

		if c.gen.Load() != gen {
			_ = out.WithLock(func(w io.Writer) error {
				return termimage.DeleteKittyImage(w, newID)
			})
			return CoverLoadedMsg{owner: owner, gen: gen, stale: true}
		}

		return CoverLoadedMsg{owner: owner,
			gen:     gen,
			path:    localPath,
			imageID: newID,
			cols:    cols,
			rows:    rows,
			width:   imgW,
			height:  imgH,
			kitty:   true,
		}
	}
}

// ApplyLoaded stores a load result and resizes the placement when the pane is
// visible. Callers pass zero dimensions while the pane is hidden.
func (c *coverPreview) ApplyLoaded(msg CoverLoadedMsg, maxCols, maxRows int) tea.Cmd {
	if msg.stale || msg.gen != c.gen.Load() {
		if msg.kitty && msg.imageID != 0 {
			return c.DeleteCmd(msg.imageID)
		}
		return nil
	}

	c.loading = false
	c.err = msg.err
	if msg.err != nil {
		c.grid = nil
		c.cols = 0
		c.rows = 0
		return nil
	}

	c.path = msg.path
	c.imageWidth = msg.width
	c.imageHeight = msg.height
	c.imageID = msg.imageID
	c.cols = msg.cols
	c.rows = msg.rows
	c.grid = termimage.KittyPlaceholderGrid(msg.imageID, msg.cols, msg.rows)
	if len(c.grid) == 0 {
		c.imageID = 0
		c.cols = 0
		c.rows = 0
		c.err = errors.New("生成封面占位符失败")
		return c.DeleteCmd(msg.imageID)
	}
	return c.Reflow(maxCols, maxRows)
}

// Reflow resizes the virtual placement without retransmitting pixels.
func (c *coverPreview) Reflow(maxCols, maxRows int) tea.Cmd {
	if maxCols <= 0 || maxRows <= 0 || c.imageID == 0 ||
		c.imageWidth <= 0 || c.imageHeight <= 0 || len(c.grid) == 0 || c.out == nil {
		return nil
	}
	cols, rows := termimage.CalculateScaledDimensions(c.imageWidth, c.imageHeight, maxCols, maxRows)
	if cols == c.cols && rows == c.rows {
		return nil
	}
	grid := termimage.KittyPlaceholderGrid(c.imageID, cols, rows)
	if grid == nil {
		return nil
	}
	c.cols = cols
	c.rows = rows
	c.grid = grid

	id := c.imageID
	gen := c.gen.Load()
	layout := c.layout.Add(1)
	out := c.out
	return func() tea.Msg {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.gen.Load() != gen || c.layout.Load() != layout {
			return nil
		}
		_ = out.WithLock(func(w io.Writer) error {
			return termimage.PlaceKittyImage(w, id, cols, rows)
		})
		return nil
	}
}

// DeleteCmd removes a specific terminal-side image.
func (c *coverPreview) DeleteCmd(imageID uint32) tea.Cmd {
	out := c.out
	if imageID == 0 || out == nil || !c.capable {
		return nil
	}
	return func() tea.Msg {
		c.mu.Lock()
		defer c.mu.Unlock()
		_ = out.WithLock(func(w io.Writer) error {
			return termimage.DeleteKittyImage(w, imageID)
		})
		return nil
	}
}

// Render builds the bordered cover box. Image grid lines are padded outside
// their SGR span so the image id color does not leak into the border.
func (c *coverPreview) Render(imageWidth int) string {
	if imageWidth <= 0 {
		return ""
	}
	lines := make([]string, 0, 8)
	if len(c.grid) > 0 && c.cols > 0 {
		cols := c.cols
		if cols > imageWidth {
			cols = imageWidth
		}
		lead := (imageWidth - cols) / 2
		trail := imageWidth - lead - cols
		for _, gridLine := range c.grid {
			lines = append(lines, strings.Repeat(" ", lead)+gridLine+strings.Repeat(" ", trail))
		}
	} else {
		center := lipgloss.NewStyle().Width(imageWidth).Align(lipgloss.Center)
		lines = append(lines,
			"",
			center.Render("[封面加载中]"),
			"",
			center.Render("正在获取封面..."),
			"",
		)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderColor).
		Width(imageWidth).
		Render(strings.Join(lines, "\n"))
}
