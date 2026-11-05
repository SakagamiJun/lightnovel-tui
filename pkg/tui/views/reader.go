package views

import (
	"context"
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

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	termimage "github.com/SakagamiJun/lightnovel-tui/pkg/image"
	"github.com/SakagamiJun/lightnovel-tui/pkg/reader"
	"github.com/SakagamiJun/lightnovel-tui/pkg/source"
	"github.com/SakagamiJun/lightnovel-tui/pkg/storage"
	"github.com/SakagamiJun/lightnovel-tui/pkg/text"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/common"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/theme"
)

type chapterContentLoadedMsg struct {
	engine       *reader.Reader
	inlineImages []inlineImage
	gen          uint64
	err          error
}

// inlineImage is a mixed-chapter illustration prepared for the viewport.
// Pixels are streamed to the terminal once; grid is the placeholder text that
// reserves the same number of rows in the reading flow.
type inlineImage struct {
	url    string
	path   string
	width  int
	height int
	cols   int
	rows   int
	id     uint32
	grid   []string
	err    error
}

type illustrationLoadedMsg struct {
	index    int
	gen      uint64
	path     string
	width    int
	height   int
	fileSize int64
	cols     int
	rows     int
	kittyID  uint32
	kitty    bool
	stale    bool
	kittyErr error
	err      error
}

// ReaderView renders chapter content using a scrollable viewport with inline
// illustrations. Image-only chapters use a full-screen gallery; unsupported
// terminals keep the metadata modal fallback.
type ReaderView struct {
	store     *storage.Storage
	src       source.DataSource
	reader    *reader.Reader
	bookID    string
	chapterID string
	viewport  viewport.Model
	width     int
	height    int
	ready     bool
	loading   bool
	err       error

	// Illustration viewer modal state
	showImageModal bool
	galleryMode    bool
	imageIndex     int
	imageLoading   bool
	imagePath      string
	imageWidth     int
	imageHeight    int
	imageSize      int64
	imageErr       error

	// Kitty graphics state
	terminalOut   *termimage.LockedWriter
	kittyCapable  bool
	kittyImageID  uint32
	imageGen      atomic.Uint64
	imageLayout   atomic.Uint64
	imageMu       sync.Mutex
	imageGrid     []string
	imageCols     int
	imageRows     int
	imageKittyErr error

	// Mixed text/illustration state. inlineImages are prepared during chapter
	// load so placeholders can reserve stable line counts from the start.
	inlineImages []inlineImage
	inlineLayout atomic.Uint64
	chapterGen   atomic.Uint64

	// Reader color theme preset
	themeIndex int
}

// NewReaderView constructs an immersive terminal reader view.
func NewReaderView(store *storage.Storage, src source.DataSource) *ReaderView {
	return &ReaderView{
		store: store,
		src:   src,
	}
}

// SetTerminalOutput enables inline Kitty illustration rendering when the
// terminal supports Unicode image placeholders. The writer must be shared with
// the Bubble Tea renderer so graphics payloads cannot interleave with frames.
func (v *ReaderView) SetTerminalOutput(out *termimage.LockedWriter) {
	v.terminalOut = out
	v.kittyCapable = out != nil && termimage.SupportsKittyPlaceholders()
}

// OpenChapter loads and displays a chapter in viewport.
func (v *ReaderView) OpenChapter(bookID, chapterID string) tea.Cmd {
	v.bookID = bookID
	v.chapterID = chapterID
	v.loading = true
	v.err = nil
	cleanup := v.resetImageState()
	inlineCleanup := v.resetInlineState()
	gen := v.chapterGen.Add(1)
	width, height, kittyCapable := v.width, v.height, v.kittyCapable

	loadCmd := func() tea.Msg {
		r, err := reader.NewReader(v.store, bookID)
		if err != nil {
			return chapterContentLoadedMsg{err: err, gen: gen}
		}

		// Ensure chapter cached
		_, err = v.store.LoadChapter(bookID, chapterID)
		if err != nil {
			// Fetch on-demand
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			ch, err := v.src.GetChapterContent(ctx, bookID, chapterID)
			if err != nil {
				return chapterContentLoadedMsg{err: err, gen: gen}
			}
			_ = v.store.SaveChapter(ch)
		}

		if _, err = r.LoadChapter(chapterID); err != nil {
			return chapterContentLoadedMsg{engine: r, err: err, gen: gen}
		}

		msg := chapterContentLoadedMsg{engine: r, gen: gen}
		// Mixed chapters preload and transmit illustrations before the text is
		// shown, so every image block reserves a stable line count.
		if kittyCapable && len(r.Illustrations()) > 0 && !r.IsIllustrationOnly() {
			msg.inlineImages = v.prepareInlineImages(r, gen, width, height)
		}
		return msg
	}

	cmds := []tea.Cmd{loadCmd}
	if cleanup != nil {
		cmds = append(cmds, cleanup)
	}
	if inlineCleanup != nil {
		cmds = append(cmds, inlineCleanup)
	}
	if len(cmds) == 1 {
		return loadCmd
	}
	return tea.Batch(cmds...)
}

func (v *ReaderView) Init() tea.Cmd {
	return nil
}

func (v *ReaderView) SetSize(width, height int) {
	v.width = width
	v.height = height
	if !v.ready {
		v.viewport = viewport.New(width, height-2)
		v.ready = true
	} else {
		v.viewport.Width = width
		v.viewport.Height = height - 2
	}
}

// modalWidths returns the bordered modal width and its inner content width.
func (v *ReaderView) modalWidths() (modalW, innerW int) {
	modalW = v.width - 6
	if modalW > 74 {
		modalW = 74
	}
	if modalW < 30 {
		modalW = 30
	}
	if maxW := v.width - 2; modalW > maxW {
		modalW = maxW
	}
	if modalW < 16 {
		modalW = 16
	}
	innerW = modalW - 2
	if innerW < 8 {
		innerW = 8
	}
	return modalW, innerW
}

// imageArea returns the maximum placeholder grid that fits inside the modal.
func (v *ReaderView) imageArea() (maxCols, maxRows int) {
	if v.galleryMode {
		maxCols = v.width - 4
		if maxCols < 8 {
			maxCols = 8
		}
		maxRows = v.height - 4
		if maxRows < 1 {
			maxRows = 1
		}
		if maxRows > 64 {
			maxRows = 64
		}
		return maxCols, maxRows
	}
	_, innerW := v.modalWidths()
	maxCols = innerW - 2
	if maxCols < 8 {
		maxCols = 8
	}
	// Header + divider + image grid + metadata + hint + box borders.
	maxRows = v.height - 8
	if maxRows < 1 {
		maxRows = 1
	}
	if maxRows > 64 {
		maxRows = 64
	}
	return maxCols, maxRows
}

func (v *ReaderView) totalIllustrations() int {
	if v.reader == nil {
		return 0
	}
	return len(v.reader.Illustrations())
}

// beginImageLoad invalidates in-flight image work and clears the current
// illustration state. It returns the new generation token for the load.
func (v *ReaderView) beginImageLoad(index int) uint64 {
	v.imageIndex = index
	v.imageLoading = true
	v.imageErr = nil
	v.imageKittyErr = nil
	v.imagePath = ""
	v.imageWidth = 0
	v.imageHeight = 0
	v.imageSize = 0
	v.imageGrid = nil
	v.imageCols = 0
	v.imageRows = 0
	return v.imageGen.Add(1)
}

// resetImageState hides the illustration modal, invalidates in-flight loads
// and schedules deletion of the committed terminal-side image.
func (v *ReaderView) resetImageState() tea.Cmd {
	v.imageGen.Add(1)
	v.showImageModal = false
	v.galleryMode = false
	v.imageIndex = 0
	v.imageLoading = false
	v.imageErr = nil
	v.imageKittyErr = nil
	v.imagePath = ""
	v.imageWidth = 0
	v.imageHeight = 0
	v.imageSize = 0
	v.imageGrid = nil
	v.imageCols = 0
	v.imageRows = 0
	id := v.kittyImageID
	v.kittyImageID = 0
	return v.deleteKittyImageCmd(id)
}

// startImageLoad resolves the illustration path on the update goroutine and
// starts an asynchronous download/transmission job.
func (v *ReaderView) startImageLoad(index int) tea.Cmd {
	if v.reader == nil || v.loading {
		return nil
	}
	urls := v.reader.Illustrations()
	if index < 0 || index >= len(urls) {
		v.imageLoading = false
		v.imageErr = fmt.Errorf("插图索引超出范围")
		return nil
	}
	gen := v.beginImageLoad(index)
	maxCols, maxRows := v.imageArea()
	return v.loadIllustrationCmd(
		index,
		gen,
		urls[index],
		v.reader.IllustrationPath(urls[index]),
		v.kittyImageID,
		maxCols,
		maxRows,
	)
}

// loadIllustrationCmd fetches illustration metadata and ensures it is
// downloaded. When Kitty placeholders are available it streams the image to the
// terminal and returns the dimensions of the virtual placement.
func (v *ReaderView) loadIllustrationCmd(index int, gen uint64, imgURL, localPath string, previousID uint32, maxCols, maxRows int) tea.Cmd {
	return func() tea.Msg {
		if v.imageGen.Load() != gen {
			return illustrationLoadedMsg{index: index, gen: gen, stale: true}
		}

		// Download if missing
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			if err := termimage.DownloadImageToFile(ctx, imgURL, localPath); err != nil {
				if v.imageGen.Load() != gen {
					return illustrationLoadedMsg{index: index, gen: gen, stale: true}
				}
				return illustrationLoadedMsg{index: index, gen: gen, err: fmt.Errorf("下载插图失败: %w", err)}
			}
		}
		if v.imageGen.Load() != gen {
			return illustrationLoadedMsg{index: index, gen: gen, stale: true}
		}

		var imgW, imgH int
		var fileSize int64
		if fi, err := os.Stat(localPath); err == nil {
			fileSize = fi.Size()
		}
		if f, err := os.Open(localPath); err == nil {
			cfg, _, err := stdimage.DecodeConfig(f)
			_ = f.Close()
			if err == nil {
				imgW = cfg.Width
				imgH = cfg.Height
			}
		}

		msg := illustrationLoadedMsg{
			index:    index,
			gen:      gen,
			path:     localPath,
			width:    imgW,
			height:   imgH,
			fileSize: fileSize,
		}

		if !v.kittyCapable || v.terminalOut == nil || imgW <= 0 || imgH <= 0 || maxCols <= 0 || maxRows <= 0 {
			return msg
		}

		cols, rows := termimage.CalculateScaledDimensions(imgW, imgH, maxCols, maxRows)
		newID := termimage.NewKittyImageID()

		v.imageMu.Lock()
		defer v.imageMu.Unlock()
		if v.imageGen.Load() != gen {
			return illustrationLoadedMsg{index: index, gen: gen, stale: true}
		}

		transmitErr := v.terminalOut.WithLock(func(w io.Writer) error {
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
			msg.kittyErr = transmitErr
			return msg
		}

		// The modal may have been closed or navigated while the upload was in
		// progress; clean up the just-created image in that case.
		if v.imageGen.Load() != gen {
			_ = v.terminalOut.WithLock(func(w io.Writer) error {
				return termimage.DeleteKittyImage(w, newID)
			})
			return illustrationLoadedMsg{index: index, gen: gen, stale: true}
		}

		msg.kitty = true
		msg.kittyID = newID
		msg.cols = cols
		msg.rows = rows
		return msg
	}
}

// deleteKittyImageCmd removes a specific terminal-side image. Deletion is
// serialized with uploads through imageMu.
func (v *ReaderView) deleteKittyImageCmd(imageID uint32) tea.Cmd {
	out := v.terminalOut
	if out == nil || imageID == 0 || !v.kittyCapable {
		return nil
	}
	return func() tea.Msg {
		v.imageMu.Lock()
		defer v.imageMu.Unlock()
		_ = out.WithLock(func(w io.Writer) error {
			return termimage.DeleteKittyImage(w, imageID)
		})
		return nil
	}
}

// inlineImageAreaFor returns the image grid budget used inside the reading
// flow. Images are a little inset from the viewport edges and never consume
// more than forty rows, keeping the prose reachable.
func (v *ReaderView) inlineImageAreaFor(width, height int) (maxCols, maxRows int) {
	maxCols = width - 4
	if maxCols < 16 {
		maxCols = 16
	}
	maxRows = height - 4
	if maxRows < 6 {
		maxRows = 6
	}
	if maxRows > 40 {
		maxRows = 40
	}
	return maxCols, maxRows
}

// prepareInlineImages downloads, decodes and transmits every illustration in a
// mixed chapter. It runs inside the chapter load command so the viewport can be
// laid out with known image dimensions and stable row counts.
func (v *ReaderView) prepareInlineImages(r *reader.Reader, gen uint64, width, height int) []inlineImage {
	urls := r.Illustrations()
	if len(urls) == 0 || !v.kittyCapable || v.terminalOut == nil {
		return nil
	}
	maxCols, maxRows := v.inlineImageAreaFor(width, height)
	images := make([]inlineImage, 0, len(urls))
	created := make([]uint32, 0, len(urls))

	for _, imgURL := range urls {
		if v.chapterGen.Load() != gen {
			v.deleteKittyImagesNow(created)
			return nil
		}

		img := inlineImage{url: imgURL, path: r.IllustrationPath(imgURL)}
		if _, err := os.Stat(img.path); err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			err = termimage.DownloadImageToFile(ctx, imgURL, img.path)
			cancel()
			if err != nil {
				img.err = fmt.Errorf("下载插图失败: %w", err)
				images = append(images, img)
				continue
			}
		}

		f, err := os.Open(img.path)
		if err != nil {
			img.err = err
			images = append(images, img)
			continue
		}
		cfg, _, decodeErr := stdimage.DecodeConfig(f)
		_ = f.Close()
		if decodeErr != nil {
			img.err = fmt.Errorf("解析插图尺寸失败: %w", decodeErr)
			images = append(images, img)
			continue
		}
		if cfg.Width <= 0 || cfg.Height <= 0 {
			img.err = fmt.Errorf("插图尺寸无效")
			images = append(images, img)
			continue
		}

		img.width = cfg.Width
		img.height = cfg.Height
		cols, rows := termimage.CalculateScaledDimensions(img.width, img.height, maxCols, maxRows)
		if cols <= 0 || rows <= 0 {
			img.err = fmt.Errorf("插图缩放尺寸无效")
			images = append(images, img)
			continue
		}
		img.cols = cols
		img.rows = rows
		newID := termimage.NewKittyImageID()

		v.imageMu.Lock()
		if v.chapterGen.Load() != gen {
			v.imageMu.Unlock()
			v.deleteKittyImagesNow(created)
			return nil
		}
		transmitErr := v.terminalOut.WithLock(func(w io.Writer) error {
			if err := termimage.TransmitKittyImage(w, img.path, newID, cols, rows); err != nil {
				_ = termimage.DeleteKittyImage(w, newID)
				return err
			}
			return nil
		})
		v.imageMu.Unlock()
		if transmitErr != nil {
			img.err = transmitErr
			images = append(images, img)
			continue
		}
		grid := termimage.KittyPlaceholderGrid(newID, cols, rows)
		if grid == nil {
			v.deleteKittyImagesNow([]uint32{newID})
			img.err = fmt.Errorf("生成插图占位符失败")
			images = append(images, img)
			continue
		}

		img.id = newID
		img.grid = grid
		created = append(created, newID)
		images = append(images, img)
	}

	if v.chapterGen.Load() != gen {
		v.deleteKittyImagesNow(created)
		return nil
	}
	return images
}

func (v *ReaderView) deleteKittyImagesNow(ids []uint32) {
	out := v.terminalOut
	if out == nil || len(ids) == 0 {
		return
	}
	v.imageMu.Lock()
	defer v.imageMu.Unlock()
	_ = out.WithLock(func(w io.Writer) error {
		for _, id := range ids {
			if id != 0 {
				_ = termimage.DeleteKittyImage(w, id)
			}
		}
		return nil
	})
}

func (v *ReaderView) deleteKittyImagesCmd(ids []uint32) tea.Cmd {
	if len(ids) == 0 || v.terminalOut == nil {
		return nil
	}
	copied := append([]uint32(nil), ids...)
	return func() tea.Msg {
		v.deleteKittyImagesNow(copied)
		return nil
	}
}

// resetInlineState clears mixed-chapter images and schedules terminal-side
// cleanup of their Kitty ids.
func (v *ReaderView) resetInlineState() tea.Cmd {
	v.inlineLayout.Add(1)
	ids := make([]uint32, 0, len(v.inlineImages))
	for _, img := range v.inlineImages {
		if img.id != 0 {
			ids = append(ids, img.id)
		}
	}
	v.inlineImages = nil
	return v.deleteKittyImagesCmd(ids)
}

// refreshInlineViewportContent lays out prose and image blocks in source order.
func (v *ReaderView) refreshInlineViewportContent() {
	lines := v.reader.Lines()
	curTheme := v.currentTheme()
	textStyle := lipgloss.NewStyle().Foreground(curTheme.Text)
	visual := make([]string, 0, len(lines)*2)
	imageIdx := -1

	for _, line := range lines {
		if isIllustrationBadge(line) {
			imageIdx++
			if imageIdx < len(v.inlineImages) {
				visual = append(visual, v.renderInlineImageBlock(imageIdx)...)
				visual = append(visual, "")
				continue
			}
		}
		visual = append(visual, textStyle.Render(line), "")
	}
	v.viewport.SetContent(strings.Join(visual, "\n"))
}

func (v *ReaderView) renderInlineImageBlock(index int) []string {
	img := &v.inlineImages[index]
	curTheme := v.currentTheme()
	center := func(s string) string {
		width := lipgloss.Width(s)
		if width >= v.width {
			return s
		}
		return strings.Repeat(" ", (v.width-width)/2) + s
	}

	if img.err != nil {
		msg := lipgloss.NewStyle().
			Foreground(theme.AccentRose).
			Render(fmt.Sprintf("[插图 %d/%d 加载失败] %v", index+1, len(v.inlineImages), img.err))
		return []string{"", center(theme.TruncateANSI(msg, v.width, "...")), ""}
	}

	if len(img.grid) == 0 || img.cols <= 0 {
		rows := img.rows
		if rows < 3 {
			rows = 3
		}
		lines := make([]string, rows)
		lines[rows/2] = center(lipgloss.NewStyle().
			Foreground(curTheme.Muted).
			Render(fmt.Sprintf("[插图 %d/%d 加载中...]", index+1, len(v.inlineImages))))
		return lines
	}

	cols := img.cols
	if cols > v.width {
		cols = v.width
	}
	lines := make([]string, 0, len(img.grid)+1)
	for _, gridLine := range img.grid {
		left := (v.width - cols) / 2
		if left < 0 {
			left = 0
		}
		right := v.width - left - cols
		if right < 0 {
			right = 0
		}
		lines = append(lines, strings.Repeat(" ", left)+gridLine+strings.Repeat(" ", right))
	}
	caption := lipgloss.NewStyle().
		Foreground(curTheme.Muted).
		Render(fmt.Sprintf(" 第 %d/%d 张插图 ", index+1, len(v.inlineImages)))
	lines = append(lines, center(caption))
	return lines
}

// reflowInlineImages recomputes mixed-chapter image grids after a resize and
// re-places the existing image ids without retransmitting pixels.
func (v *ReaderView) reflowInlineImages() tea.Cmd {
	if len(v.inlineImages) == 0 || !v.kittyCapable || v.terminalOut == nil {
		return nil
	}
	maxCols, maxRows := v.inlineImageAreaFor(v.width, v.height)
	type placement struct {
		id         uint32
		cols, rows int
	}
	placements := make([]placement, 0, len(v.inlineImages))
	changed := false
	for i := range v.inlineImages {
		img := &v.inlineImages[i]
		if img.id == 0 || img.width <= 0 || img.height <= 0 {
			continue
		}
		cols, rows := termimage.CalculateScaledDimensions(img.width, img.height, maxCols, maxRows)
		if cols == img.cols && rows == img.rows {
			continue
		}
		grid := termimage.KittyPlaceholderGrid(img.id, cols, rows)
		if grid == nil {
			continue
		}
		img.cols = cols
		img.rows = rows
		img.grid = grid
		placements = append(placements, placement{id: img.id, cols: cols, rows: rows})
		changed = true
	}
	if !changed {
		return nil
	}

	percent := v.viewport.ScrollPercent()
	v.refreshViewportContent()
	v.viewport.SetYOffset(int(percent * float64(v.viewport.TotalLineCount())))

	gen := v.chapterGen.Load()
	layout := v.inlineLayout.Add(1)
	out := v.terminalOut
	return func() tea.Msg {
		v.imageMu.Lock()
		defer v.imageMu.Unlock()
		if v.chapterGen.Load() != gen || v.inlineLayout.Load() != layout {
			return nil
		}
		_ = out.WithLock(func(w io.Writer) error {
			for _, p := range placements {
				if err := termimage.PlaceKittyImage(w, p.id, p.cols, p.rows); err != nil {
					return err
				}
			}
			return nil
		})
		return nil
	}
}

// reflowKittyImage resizes the virtual placement after a terminal resize
// without retransmitting pixel data. It also refreshes the placeholder grid.
func (v *ReaderView) reflowKittyImage() tea.Cmd {
	if !v.showImageModal || !v.kittyCapable || v.terminalOut == nil ||
		v.kittyImageID == 0 || len(v.imageGrid) == 0 ||
		v.imageWidth <= 0 || v.imageHeight <= 0 {
		return nil
	}

	maxCols, maxRows := v.imageArea()
	cols, rows := termimage.CalculateScaledDimensions(v.imageWidth, v.imageHeight, maxCols, maxRows)
	if cols == v.imageCols && rows == v.imageRows {
		return nil
	}
	grid := termimage.KittyPlaceholderGrid(v.kittyImageID, cols, rows)
	if grid == nil {
		return nil
	}
	v.imageCols = cols
	v.imageRows = rows
	v.imageGrid = grid

	id := v.kittyImageID
	gen := v.imageGen.Load()
	layout := v.imageLayout.Add(1)
	out := v.terminalOut
	return func() tea.Msg {
		v.imageMu.Lock()
		defer v.imageMu.Unlock()
		if v.imageGen.Load() != gen || v.imageLayout.Load() != layout {
			return nil
		}
		_ = out.WithLock(func(w io.Writer) error {
			return termimage.PlaceKittyImage(w, id, cols, rows)
		})
		return nil
	}
}

func (v *ReaderView) currentTheme() theme.ReaderTheme {
	return theme.GetReaderTheme(theme.ReaderThemeID(v.themeIndex))
}

// saveProgress persists the current reading position. Gallery chapters use
// the image index, text chapters use the viewport line offset.
func (v *ReaderView) saveProgress() {
	if v.reader == nil {
		return
	}
	p := &reader.ReadingProgress{
		BookID:     v.bookID,
		ChapterID:  v.chapterID,
		LastReadAt: time.Now().Unix(),
	}
	if v.galleryMode {
		p.ImageIndex = v.imageIndex
	} else {
		p.LineIndex = v.viewport.YOffset
	}
	_ = v.reader.SaveProgress(p)
}

// exitReaderCmd saves the bookmark and removes inline/gallery images before
// returning to the catalog.
func (v *ReaderView) exitReaderCmd() tea.Cmd {
	v.saveProgress()
	v.chapterGen.Add(1)
	modalCleanup := v.resetImageState()
	inlineCleanup := v.resetInlineState()
	return func() tea.Msg {
		if modalCleanup != nil {
			_ = modalCleanup()
		}
		if inlineCleanup != nil {
			_ = inlineCleanup()
		}
		return common.SwitchViewMsg{
			Target: common.ViewCatalog,
			BookID: v.bookID,
		}
	}
}

// isIllustrationBadge matches both Simplified and Traditional badge prefixes.
func isIllustrationBadge(line string) bool {
	return strings.HasPrefix(line, "[插图:") || strings.HasPrefix(line, "[插圖:")
}

func (v *ReaderView) refreshViewportContent() {
	if v.reader == nil {
		return
	}
	if len(v.inlineImages) > 0 {
		v.refreshInlineViewportContent()
		return
	}
	lines := v.reader.Lines()
	curTheme := v.currentTheme()
	textStyle := lipgloss.NewStyle().Foreground(curTheme.Text)

	styledLines := make([]string, len(lines))
	for i, l := range lines {
		if isIllustrationBadge(l) {
			styledLines[i] = lipgloss.NewStyle().Bold(true).Foreground(curTheme.Accent).Render(l)
		} else {
			styledLines[i] = textStyle.Render(l)
		}
	}
	v.viewport.SetContent(strings.Join(styledLines, "\n\n"))
}

func (v *ReaderView) Update(msg tea.Msg) (*ReaderView, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case chapterContentLoadedMsg:
		if msg.gen != 0 && msg.gen != v.chapterGen.Load() {
			return v, nil
		}
		v.loading = false
		v.err = msg.err
		v.galleryMode = false
		v.inlineImages = msg.inlineImages
		if msg.engine != nil {
			v.reader = msg.engine
		}
		if v.err == nil && v.reader != nil {
			var saved *reader.ReadingProgress
			if p, err := v.reader.LoadProgress(); err == nil && p != nil && p.ChapterID == v.chapterID {
				saved = p
			}

			// A chapter containing only illustrations opens directly as a
			// full-screen gallery, with no [i] step.
			if v.kittyCapable && v.reader.IsIllustrationOnly() {
				v.inlineImages = nil
				v.showImageModal = true
				v.galleryMode = true
				index := 0
				if saved != nil && saved.ImageIndex > 0 {
					index = saved.ImageIndex
				}
				if total := v.totalIllustrations(); index >= total {
					index = total - 1
				}
				if index < 0 {
					index = 0
				}
				return v, v.startImageLoad(index)
			}

			v.refreshViewportContent()
			v.viewport.GotoTop()
			if saved != nil {
				// Scroll to saved position
				v.viewport.SetYOffset(saved.LineIndex)
			}
			if cmd := v.reflowInlineImages(); cmd != nil {
				return v, cmd
			}
		}
		return v, nil

	case illustrationLoadedMsg:
		if msg.stale || msg.gen != v.imageGen.Load() || !v.showImageModal || msg.index != v.imageIndex {
			if msg.kitty && msg.kittyID != 0 {
				return v, v.deleteKittyImageCmd(msg.kittyID)
			}
			return v, nil
		}

		v.imageLoading = false
		v.imageErr = msg.err
		v.imageKittyErr = msg.kittyErr
		if msg.err != nil {
			return v, nil
		}

		v.imagePath = msg.path
		v.imageWidth = msg.width
		v.imageHeight = msg.height
		v.imageSize = msg.fileSize
		v.imageGrid = nil
		v.imageCols = 0
		v.imageRows = 0

		if msg.kitty && msg.cols > 0 && msg.rows > 0 {
			grid := termimage.KittyPlaceholderGrid(msg.kittyID, msg.cols, msg.rows)
			if grid != nil {
				v.kittyImageID = msg.kittyID
				v.imageCols = msg.cols
				v.imageRows = msg.rows
				v.imageGrid = grid
				if cmd := v.reflowKittyImage(); cmd != nil {
					return v, cmd
				}
			} else {
				return v, v.deleteKittyImageCmd(msg.kittyID)
			}
		}
		return v, nil

	case tea.ResumeMsg:
		// Suspending releases the alternate screen, which clears Kitty image
		// data. Reload mixed chapters so all inline images are retransmitted.
		if len(v.inlineImages) > 0 {
			v.saveProgress()
			return v, v.OpenChapter(v.bookID, v.chapterID)
		}
		// Re-upload the current illustration when a gallery or modal is still
		// open. Restarting also covers a load that was interrupted by suspend.
		if v.showImageModal && (v.kittyImageID != 0 || v.galleryMode) {
			return v, v.startImageLoad(v.imageIndex)
		}

	case tea.WindowSizeMsg:
		if cmd := v.reflowInlineImages(); cmd != nil {
			return v, cmd
		}
		if cmd := v.reflowKittyImage(); cmd != nil {
			return v, cmd
		}

	case tea.KeyMsg:
		// Modal interaction when viewing illustration
		if v.showImageModal {
			if v.galleryMode {
				switch msg.String() {
				case "esc", "q":
					return v, v.exitReaderCmd()
				case "i", "I":
					return v, nil
				case "left", "h", "[", "p", "up", "k":
					if v.imageIndex > 0 {
						cmd := v.startImageLoad(v.imageIndex - 1)
						v.saveProgress()
						return v, cmd
					}
					return v, nil
				case "right", "l", "]", "n", "down", "j":
					if v.imageIndex < v.totalIllustrations()-1 {
						cmd := v.startImageLoad(v.imageIndex + 1)
						v.saveProgress()
						return v, cmd
					}
					return v, nil
				case "enter", " ", "space", "o", "O":
					if v.imagePath != "" {
						_ = termimage.OpenInSystemViewer(v.imagePath)
					}
					return v, nil
				default:
					return v, nil
				}
			}

			switch msg.String() {
			case "esc", "i", "I", "q":
				return v, v.resetImageState()
			case "left", "h", "[", "p", "up", "k":
				if v.imageIndex > 0 {
					return v, v.startImageLoad(v.imageIndex - 1)
				}
				return v, nil
			case "right", "l", "]", "n", "down", "j":
				if v.imageIndex < v.totalIllustrations()-1 {
					return v, v.startImageLoad(v.imageIndex + 1)
				}
				return v, nil
			case "enter", " ", "space", "o", "O":
				if v.imagePath != "" {
					_ = termimage.OpenInSystemViewer(v.imagePath)
				}
				return v, nil
			default:
				return v, nil
			}
		}

		// Regular text reading interaction
		switch msg.String() {
		case "i", "I":
			// Unsupported terminals keep the metadata modal as a fallback.
			if !v.kittyCapable && v.err == nil && !v.loading && v.totalIllustrations() > 0 {
				v.showImageModal = true
				return v, v.startImageLoad(0)
			}

		case "c", "C":
			v.themeIndex = (v.themeIndex + 1) % len(theme.ReaderThemes)
			yOffset := v.viewport.YOffset
			v.refreshViewportContent()
			v.viewport.SetYOffset(yOffset)
			return v, nil

		case "t", "T":
			if v.reader != nil {
				v.reader.ToggleTraditional()
				yOffset := v.viewport.YOffset
				v.refreshViewportContent()
				v.viewport.SetYOffset(yOffset)
			}
			return v, nil

		case "[", "p":
			if v.reader != nil {
				if prevID, _, ok := v.reader.PrevChapter(); ok {
					v.saveProgress()
					return v, v.OpenChapter(v.bookID, prevID)
				}
			}
			return v, nil

		case "]", "n":
			if v.reader != nil {
				if nextID, _, ok := v.reader.NextChapter(); ok {
					v.saveProgress()
					return v, v.OpenChapter(v.bookID, nextID)
				}
			}
			return v, nil

		case "esc":
			return v, v.exitReaderCmd()
		}
	}

	var vpCmd tea.Cmd
	v.viewport, vpCmd = v.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return v, tea.Batch(cmds...)
}

func (v *ReaderView) View() string {
	if v.loading {
		return "\n[加载中] 正在获取正文内容，请稍候..."
	}
	if v.err != nil {
		return fmt.Sprintf("\n[错误] 加载正文出错: %v\n按 [Esc] 返回目录", v.err)
	}

	if v.showImageModal {
		if v.galleryMode {
			return v.renderGalleryView()
		}
		return v.renderImageModal()
	}

	curTheme := v.currentTheme()

	title := ""
	if v.reader != nil && v.reader.CurrentChapter() != nil {
		title = v.reader.CurrentChapter().Title
	}
	if v.reader != nil && v.reader.IsTraditional() {
		title = text.ToTraditional(title)
	}

	illuCount := 0
	if v.reader != nil {
		illuCount = len(v.reader.Illustrations())
	}

	// 1. Top Header Bar (1 line)
	leftTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(curTheme.Text).
		Render(" " + title)

	rightElements := make([]string, 0)
	if illuCount > 0 {
		label := fmt.Sprintf("[插图:%d张]", illuCount)
		if v.kittyCapable {
			if len(v.inlineImages) > 0 {
				label += "[已内联]"
			} else {
				label += "[图文混排]"
			}
		} else {
			label += "[i]"
		}
		rightElements = append(rightElements, lipgloss.NewStyle().
			Bold(true).
			Foreground(curTheme.Accent).
			Render(label))
	}

	tradLabel := "[t]繁体"
	if v.reader != nil && v.reader.IsTraditional() {
		tradLabel = "[t]简体"
	}
	rightElements = append(rightElements, lipgloss.NewStyle().Foreground(curTheme.Muted).Render(tradLabel))
	rightElements = append(rightElements, lipgloss.NewStyle().Foreground(curTheme.Muted).Render("[c]配色"))
	rightElements = append(rightElements, lipgloss.NewStyle().Foreground(curTheme.Muted).Render("[Esc]返回目录 "))

	rightStr := strings.Join(rightElements, "  ")
	topBar := leftTitle + "  " + rightStr
	if v.width > lipgloss.Width(leftTitle)+lipgloss.Width(rightStr) {
		topGap := v.width - lipgloss.Width(leftTitle) - lipgloss.Width(rightStr)
		topBar = leftTitle + strings.Repeat(" ", topGap) + rightStr
	}
	topBar = lipgloss.NewStyle().
		Background(curTheme.BarBg).
		Width(v.width).
		Render(topBar)

	// 2. Bottom Status Bar (1 line)
	currChIdx, totalChs, _ := 1, 1, ""
	if v.reader != nil {
		currChIdx, totalChs, _ = v.reader.ChapterPosition()
	}

	totalLines := v.viewport.TotalLineCount()
	currLine := v.viewport.YOffset + 1
	if currLine > totalLines {
		currLine = totalLines
	}
	if currLine < 1 {
		currLine = 1
	}

	percent := v.viewport.ScrollPercent() * 100
	nowTime := time.Now().Format("15:04")

	statusLeft := lipgloss.NewStyle().
		Foreground(curTheme.Text).
		Render(fmt.Sprintf(" [第 %d/%d 章] %s", currChIdx, totalChs, title))

	statusMid := lipgloss.NewStyle().
		Bold(true).
		Foreground(curTheme.Accent).
		Render(fmt.Sprintf("第 %d/%d 行 (%.1f%%)", currLine, totalLines, percent))

	statusRight := lipgloss.NewStyle().
		Foreground(curTheme.Muted).
		Render(fmt.Sprintf("[[/]]换章  •  配色:%s[c]  •  %s ", curTheme.Name, nowTime))

	statusMidContent := statusLeft + "  •  " + statusMid
	bottomBar := statusMidContent + "  " + statusRight
	if v.width > lipgloss.Width(statusMidContent)+lipgloss.Width(statusRight) {
		bottomGap := v.width - lipgloss.Width(statusMidContent) - lipgloss.Width(statusRight)
		bottomBar = statusMidContent + strings.Repeat(" ", bottomGap) + statusRight
	}
	bottomBar = lipgloss.NewStyle().
		Background(curTheme.BarBg).
		Width(v.width).
		Render(bottomBar)

	return topBar + "\n" + v.viewport.View() + "\n" + bottomBar
}

// renderGalleryView shows an illustration-only chapter as a full-screen image
// gallery, replacing the text viewport entirely.
func (v *ReaderView) renderGalleryView() string {
	width := v.width
	if width < 20 {
		width = 20
	}
	height := v.height
	if height < 4 {
		height = 4
	}
	curTheme := v.currentTheme()
	total := v.totalIllustrations()

	title := ""
	if v.reader != nil && v.reader.CurrentChapter() != nil {
		title = v.reader.CurrentChapter().Title
	}
	if v.reader != nil && v.reader.IsTraditional() {
		title = text.ToTraditional(title)
	}

	headerText := fmt.Sprintf(" 插图欣赏  [%d/%d]  %s", v.imageIndex+1, total, title)
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(curTheme.Text).
		Background(curTheme.BarBg).
		Width(width).
		Render(theme.TruncateANSI(headerText, width, "..."))

	footerText := " [←/→] 上一张/下一张  •  [o] 系统预览  •  [Esc] 返回目录"
	footer := lipgloss.NewStyle().
		Foreground(curTheme.Muted).
		Background(curTheme.BarBg).
		Width(width).
		Render(theme.TruncateANSI(footerText, width, "..."))

	areaHeight := height - 2
	if areaHeight < 1 {
		areaHeight = 1
	}

	center := func(s string) string {
		lineWidth := lipgloss.Width(s)
		if lineWidth >= width {
			return s
		}
		left := (width - lineWidth) / 2
		return strings.Repeat(" ", left) + s
	}

	body := make([]string, 0, areaHeight)
	switch {
	case v.imageLoading:
		body = append(body, center(lipgloss.NewStyle().
			Foreground(theme.TextMuted).
			Render("[加载中] 正在获取插图，请稍候...")))
	case v.imageErr != nil:
		body = append(body, center(theme.TruncateANSI(lipgloss.NewStyle().
			Foreground(theme.AccentRose).
			Render(fmt.Sprintf("[错误] 插图加载失败: %v", v.imageErr)), width, "...")))
	case v.imageKittyErr != nil:
		body = append(body, center(theme.TruncateANSI(lipgloss.NewStyle().
			Foreground(theme.AccentRose).
			Render(fmt.Sprintf("[提示] Kitty 渲染失败: %v", v.imageKittyErr)), width, "...")))
		if v.imagePath != "" {
			body = append(body, "", center(lipgloss.NewStyle().
				Foreground(theme.TextMuted).
				Render("按 [o] 使用系统预览打开高清原图")))
		}
	case len(v.imageGrid) > 0 && v.imageCols > 0:
		cols := v.imageCols
		if cols > width {
			cols = width
		}
		for _, gridLine := range v.imageGrid {
			left := (width - cols) / 2
			if left < 0 {
				left = 0
			}
			right := width - left - cols
			if right < 0 {
				right = 0
			}
			body = append(body, strings.Repeat(" ", left)+gridLine+strings.Repeat(" ", right))
		}
	default:
		body = append(body, center(lipgloss.NewStyle().
			Foreground(theme.TextMuted).
			Render(fmt.Sprintf("插图 %d/%d，按 [o] 使用系统预览", v.imageIndex+1, total))))
	}

	top := (areaHeight - len(body)) / 2
	if top < 0 {
		top = 0
	}
	lines := make([]string, 0, height)
	lines = append(lines, header)
	for i := 0; i < areaHeight; i++ {
		bodyIndex := i - top
		if bodyIndex >= 0 && bodyIndex < len(body) {
			lines = append(lines, body[bodyIndex])
		} else {
			lines = append(lines, "")
		}
	}
	lines = append(lines, footer)
	return strings.Join(lines, "\n")
}

func (v *ReaderView) renderImageModal() string {
	modalW, innerW := v.modalWidths()
	totalImages := v.totalIllustrations()

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.TextWhite)

	hintStyle := lipgloss.NewStyle().
		Foreground(theme.TextMuted)

	counterStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#38BDF8"))

	header := fmt.Sprintf(" %s %s  %s",
		titleStyle.Render("[插图查看器]"),
		counterStyle.Render(fmt.Sprintf("[%d/%d]", v.imageIndex+1, totalImages)),
		hintStyle.Render("• [←/→] 翻页 • [Enter/o] 预览 • [Esc/i] 关闭"),
	)
	headerTrunc := theme.TruncateANSI(header, innerW, "...")
	divider := lipgloss.NewStyle().Foreground(theme.PrimaryColor).Render(strings.Repeat("─", innerW))

	var body string
	switch {
	case v.imageLoading:
		body = truncateBlock(lipgloss.NewStyle().
			Foreground(theme.TextMuted).
			Render("  [加载中] 正在拉取插图数据，请稍候..."), innerW)
	case v.imageErr != nil:
		body = truncateBlock(lipgloss.NewStyle().
			Foreground(theme.AccentRose).
			Render(fmt.Sprintf("  [错误] 插图加载失败: %v\n  按 [Esc] 返回阅读", v.imageErr)), innerW)
	case len(v.imageGrid) > 0 && v.imageCols > 0 &&
		v.imageCols <= innerW && len(v.imageGrid) <= v.height-6:
		body = v.renderKittyImageBody(innerW)
	default:
		body = v.renderImageInfoBody(innerW)
	}

	modalContent := headerTrunc + "\n" + divider + "\n" + body

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.PrimaryColor).
		Width(modalW).
		Padding(0, 1).
		Render(modalContent)

	return lipgloss.Place(v.width, v.height, lipgloss.Center, lipgloss.Center, box)
}

// renderKittyImageBody builds the placeholder grid plus a compact metadata and
// key-hint footer. Each grid line is padded outside the SGR color span so the
// image id color never leaks into the modal chrome.
func (v *ReaderView) renderKittyImageBody(innerW int) string {
	cols := v.imageCols
	if cols <= 0 {
		cols = innerW
	}
	if cols > innerW {
		cols = innerW
	}

	lines := make([]string, 0, len(v.imageGrid)+2)
	for _, gridLine := range v.imageGrid {
		lead := (innerW - cols) / 2
		trail := innerW - lead - cols
		lines = append(lines, strings.Repeat(" ", lead)+gridLine+strings.Repeat(" ", trail))
	}

	lines = append(lines, v.imageMetaLine(innerW))

	hint := " [←/→] 翻页  •  [Enter/o] 系统 Quick Look 高清预览  •  [Esc/i] 关闭"
	if v.imageKittyErr != nil {
		hint = fmt.Sprintf(" [提示] Kitty 渲染失败: %v；按 [o] 使用系统预览", v.imageKittyErr)
	}
	lines = append(lines, theme.TruncateANSI(
		lipgloss.NewStyle().Foreground(theme.TextDim).Render(hint), innerW, "..."))

	return strings.Join(lines, "\n")
}

func (v *ReaderView) imageMetaLine(width int) string {
	specStr := "高清插画文件"
	if v.imageWidth > 0 && v.imageHeight > 0 {
		specStr = fmt.Sprintf("%d × %d 像素", v.imageWidth, v.imageHeight)
	}

	line := fmt.Sprintf(" 第 %d/%d 张  •  %s  •  %s  •  Kitty 图形协议",
		v.imageIndex+1, v.totalIllustrations(), specStr, formatImageSize(v.imageSize))
	return theme.TruncateANSI(
		lipgloss.NewStyle().Foreground(theme.TextMuted).Render(line), width, "...")
}

func (v *ReaderView) renderImageInfoBody(innerW int) string {
	specStr := "高清插画文件"
	if v.imageWidth > 0 && v.imageHeight > 0 {
		specStr = fmt.Sprintf("%d × %d 像素 (原生高清插画)", v.imageWidth, v.imageHeight)
	}

	renderMode := "元数据模式 (终端未启用 Kitty 图形协议)"
	if v.kittyCapable {
		renderMode = "Kitty 图形协议 (Unicode Placeholder)"
	}
	if v.imageKittyErr != nil {
		renderMode = fmt.Sprintf("Kitty 渲染失败，已回退元数据模式: %v", v.imageKittyErr)
	}

	labelStyle := lipgloss.NewStyle().Foreground(theme.PrimaryLight).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(theme.TextWhite)
	statusStyle := lipgloss.NewStyle().Foreground(theme.AccentEmerald).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(theme.TextMuted)
	dimStyle := lipgloss.NewStyle().Foreground(theme.TextDim)

	lines := []string{
		fmt.Sprintf("  %s %s", labelStyle.Render("插图序号:"), valueStyle.Render(fmt.Sprintf("第 %d / %d 张插图", v.imageIndex+1, v.totalIllustrations()))),
		fmt.Sprintf("  %s %s", labelStyle.Render("图片规格:"), valueStyle.Render(specStr)),
		fmt.Sprintf("  %s %s", labelStyle.Render("文件大小:"), valueStyle.Render(formatImageSize(v.imageSize))),
		fmt.Sprintf("  %s %s", labelStyle.Render("渲染模式:"), statusStyle.Render(renderMode)),
		"",
		lipgloss.NewStyle().Foreground(theme.BorderColor).Render("  " + strings.Repeat("┄", max(innerW-4, 1))),
		"",
		lipgloss.NewStyle().Foreground(theme.AccentAmber).Bold(true).Render("  操作指南:"),
		mutedStyle.Render("  • 按 [Enter] / [Space] / [o] 调出系统 Quick Look 高清预览"),
		dimStyle.Render("    (macOS 原生 Quick Look 秒级弹出，Retina 缩放与全屏)"),
		mutedStyle.Render("  • 按 [← / →] 或 [h / l] 切换上一张 / 下一张插图"),
		mutedStyle.Render("  • 按 [Esc] 或 [i] 关闭插图查看器，返回沉浸阅读"),
	}
	if v.imageKittyErr != nil {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.AccentRose).
			Render(fmt.Sprintf("  [提示] %v", v.imageKittyErr)))
	}

	maxBodyLines := v.height - 6
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}
	if len(lines) > maxBodyLines {
		lines = lines[:maxBodyLines]
	}
	return truncateBlock(strings.Join(lines, "\n"), innerW)
}

func formatImageSize(size int64) string {
	if size <= 0 {
		return "已下载"
	}
	if size >= 1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024)
}

func truncateBlock(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = theme.TruncateANSI(lines[i], width, "...")
	}
	return strings.Join(lines, "\n")
}
