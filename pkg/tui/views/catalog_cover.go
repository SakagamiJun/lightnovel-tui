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
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/theme"
)

const (
	// catalogCoverMinWidth is the minimum terminal width for reserving a
	// right-side cover column.
	catalogCoverMinWidth = 80
	// catalogCoverPaneWidth includes the border of the cover box.
	catalogCoverPaneWidth = 24
	// catalogCoverImageWidth is the placeholder grid budget inside the border.
	catalogCoverImageWidth = catalogCoverPaneWidth - 2
	// catalogCoverMinListWidth keeps the chapter list readable when the cover
	// column is visible.
	catalogCoverMinListWidth = 40
)

var (
	errCatalogCoverUnavailable = errors.New("当前终端不支持封面内联渲染")
	errCatalogCoverEmpty       = errors.New("没有可用的封面地址或本地缓存")
)

// coverPreview owns the single terminal image used by the catalog page.
type coverPreview struct {
	out     *termimage.LockedWriter
	capable bool

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

type catalogCoverLoadedMsg struct {
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

// SetTerminalOutput enables the catalog cover preview when the terminal
// supports Kitty Unicode placeholders.
func (v *CatalogView) SetTerminalOutput(out *termimage.LockedWriter) {
	v.cover.out = out
	v.cover.capable = out != nil && termimage.SupportsKittyPlaceholders()
}

// coverPaneWidth returns the width reserved for the cover column, or zero when
// the current book, terminal or layout cannot show a cover.
func (v *CatalogView) coverPaneWidth() int {
	c := &v.cover
	if !c.capable || c.out == nil || v.store == nil || v.detail == nil || c.err != nil {
		return 0
	}
	if c.url == "" && c.path == "" {
		return 0
	}
	if v.width < catalogCoverMinWidth || v.height < 8 {
		return 0
	}
	fullWidth := v.width - 2
	if fullWidth-catalogCoverPaneWidth-1 < catalogCoverMinListWidth {
		return 0
	}
	return catalogCoverPaneWidth
}

// coverArea returns the maximum placeholder grid that fits in the right
// column. The cover box border and the one-line catalog header are excluded.
func (v *CatalogView) coverArea() (maxCols, maxRows int) {
	maxCols = catalogCoverImageWidth
	maxRows = v.height - 4
	if maxRows < 4 {
		maxRows = 4
	}
	if maxRows > 40 {
		maxRows = 40
	}
	return maxCols, maxRows
}

// resetCoverState hides the current cover, invalidates in-flight loads and
// schedules deletion of the terminal-side image.
func (v *CatalogView) resetCoverState() tea.Cmd {
	c := &v.cover
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
	return v.deleteCoverCmd(id)
}

// startCoverLoadIfNeeded kicks off a cover load when the pane is eligible and
// no image is currently displayed or loading.
func (v *CatalogView) startCoverLoadIfNeeded() tea.Cmd {
	c := &v.cover
	if v.coverPaneWidth() == 0 {
		return nil
	}
	if c.loading || c.imageID != 0 || len(c.grid) > 0 {
		return nil
	}
	if c.url == "" && c.path == "" {
		return nil
	}
	return v.loadCoverCmd()
}

// loadCoverCmd downloads the cover when only a URL is known, decodes its
// dimensions and streams it to the terminal. The command is generation-guarded
// so a book switch or resize can discard late results.
func (v *CatalogView) loadCoverCmd() tea.Cmd {
	c := &v.cover
	c.loading = true
	c.err = nil
	c.grid = nil
	c.cols = 0
	c.rows = 0

	gen := c.gen.Add(1)
	previousID := c.imageID
	bookID := v.bookID
	coverURL := c.url
	localPath := c.path
	maxCols, maxRows := v.coverArea()
	out := c.out
	store := v.store
	src := v.src
	capable := c.capable

	return func() tea.Msg {
		if c.gen.Load() != gen {
			return catalogCoverLoadedMsg{gen: gen, stale: true}
		}
		if store == nil {
			return catalogCoverLoadedMsg{gen: gen, err: errCatalogCoverUnavailable}
		}

		if localPath != "" {
			if _, err := os.Stat(localPath); err != nil {
				localPath = ""
			}
		}
		if localPath == "" {
			if coverURL == "" {
				return catalogCoverLoadedMsg{gen: gen, err: errCatalogCoverEmpty}
			}
			dl, err := downloader.NewDownloader(src, store)
			if err != nil {
				return catalogCoverLoadedMsg{gen: gen, err: fmt.Errorf("初始化封面下载器失败: %w", err)}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			path, err := dl.DownloadCover(ctx, bookID, coverURL)
			cancel()
			if err != nil {
				return catalogCoverLoadedMsg{gen: gen, err: fmt.Errorf("下载封面失败: %w", err)}
			}
			localPath = path
		}

		if localPath == "" {
			return catalogCoverLoadedMsg{gen: gen, err: errCatalogCoverEmpty}
		}
		if c.gen.Load() != gen {
			return catalogCoverLoadedMsg{gen: gen, stale: true}
		}
		if !capable || out == nil {
			return catalogCoverLoadedMsg{gen: gen, err: errCatalogCoverUnavailable}
		}

		var imgW, imgH int
		f, err := os.Open(localPath)
		if err != nil {
			return catalogCoverLoadedMsg{gen: gen, err: err}
		}
		cfg, _, decodeErr := stdimage.DecodeConfig(f)
		_ = f.Close()
		if decodeErr != nil {
			return catalogCoverLoadedMsg{gen: gen, err: fmt.Errorf("解析封面尺寸失败: %w", decodeErr)}
		}
		imgW, imgH = cfg.Width, cfg.Height
		if imgW <= 0 || imgH <= 0 {
			return catalogCoverLoadedMsg{gen: gen, err: errCatalogCoverEmpty}
		}

		cols, rows := termimage.CalculateScaledDimensions(imgW, imgH, maxCols, maxRows)
		newID := termimage.NewKittyImageID()

		c.mu.Lock()
		defer c.mu.Unlock()
		if c.gen.Load() != gen {
			return catalogCoverLoadedMsg{gen: gen, stale: true}
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
			return catalogCoverLoadedMsg{gen: gen, err: transmitErr}
		}

		if c.gen.Load() != gen {
			_ = out.WithLock(func(w io.Writer) error {
				return termimage.DeleteKittyImage(w, newID)
			})
			return catalogCoverLoadedMsg{gen: gen, stale: true}
		}

		return catalogCoverLoadedMsg{
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

// updateCover applies a cover load result. It returns a cleanup or reflow
// command when needed.
func (v *CatalogView) updateCover(msg catalogCoverLoadedMsg) tea.Cmd {
	c := &v.cover
	if msg.stale || msg.gen != c.gen.Load() {
		if msg.kitty && msg.imageID != 0 {
			return v.deleteCoverCmd(msg.imageID)
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
		return v.deleteCoverCmd(msg.imageID)
	}
	return v.reflowCover()
}

// handleCoverResize adapts the pane after a terminal resize. When the pane is
// not eligible the already-transmitted image is kept hidden in terminal memory
// so shrinking and re-expanding the window does not require a reload.
func (v *CatalogView) handleCoverResize() tea.Cmd {
	c := &v.cover
	if v.coverPaneWidth() == 0 {
		return nil
	}
	if c.imageID == 0 {
		return v.startCoverLoadIfNeeded()
	}
	return v.reflowCover()
}

// reflowCover resizes the virtual placement without retransmitting pixels.
func (v *CatalogView) reflowCover() tea.Cmd {
	c := &v.cover
	if v.coverPaneWidth() == 0 || c.imageID == 0 || c.imageWidth <= 0 || c.imageHeight <= 0 || len(c.grid) == 0 || c.out == nil {
		return nil
	}

	maxCols, maxRows := v.coverArea()
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

func (v *CatalogView) deleteCoverCmd(imageID uint32) tea.Cmd {
	c := &v.cover
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

// renderCoverPane builds the bordered right-side cover column. Image grid
// lines are padded outside their SGR span so the image id color does not leak
// into the border.
func (v *CatalogView) renderCoverPane() string {
	c := &v.cover
	lines := make([]string, 0, 8)
	if len(c.grid) > 0 && c.cols > 0 {
		cols := c.cols
		if cols > catalogCoverImageWidth {
			cols = catalogCoverImageWidth
		}
		lead := (catalogCoverImageWidth - cols) / 2
		trail := catalogCoverImageWidth - lead - cols
		for _, gridLine := range c.grid {
			lines = append(lines, strings.Repeat(" ", lead)+gridLine+strings.Repeat(" ", trail))
		}
	} else {
		center := lipgloss.NewStyle().Width(catalogCoverImageWidth).Align(lipgloss.Center)
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
		Width(catalogCoverImageWidth).
		Render(strings.Join(lines, "\n"))
}
