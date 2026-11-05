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
	err error
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

// ReaderView renders chapter content using a scrollable viewport,
// with an integrated in-terminal illustration viewer modal.
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

	loadCmd := func() tea.Msg {
		r, err := reader.NewReader(v.store, bookID)
		if err != nil {
			return chapterContentLoadedMsg{err: err}
		}
		v.reader = r

		// Ensure chapter cached
		_, err = v.store.LoadChapter(bookID, chapterID)
		if err != nil {
			// Fetch on-demand
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			ch, err := v.src.GetChapterContent(ctx, bookID, chapterID)
			if err != nil {
				return chapterContentLoadedMsg{err: err}
			}
			_ = v.store.SaveChapter(ch)
		}

		_, err = v.reader.LoadChapter(chapterID)
		return chapterContentLoadedMsg{err: err}
	}

	if cleanup != nil {
		return tea.Batch(loadCmd, cleanup)
	}
	return loadCmd
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
	if v.reader == nil {
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

func (v *ReaderView) refreshViewportContent() {
	if v.reader == nil {
		return
	}
	lines := v.reader.Lines()
	curTheme := v.currentTheme()
	textStyle := lipgloss.NewStyle().Foreground(curTheme.Text)

	styledLines := make([]string, len(lines))
	for i, l := range lines {
		if strings.HasPrefix(l, "[插图:") {
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
		v.loading = false
		v.err = msg.err
		if v.err == nil && v.reader != nil {
			v.refreshViewportContent()
			v.viewport.GotoTop()

			// Check if bookmark exists
			p, err := v.reader.LoadProgress()
			if err == nil && p != nil && p.ChapterID == v.chapterID {
				// Scroll to saved position
				v.viewport.SetYOffset(p.LineIndex)
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
		// data. Re-upload the current illustration when the modal is still open.
		if v.showImageModal && v.kittyImageID != 0 {
			return v, v.startImageLoad(v.imageIndex)
		}

	case tea.WindowSizeMsg:
		if cmd := v.reflowKittyImage(); cmd != nil {
			return v, cmd
		}

	case tea.KeyMsg:
		// Modal interaction when viewing illustration
		if v.showImageModal {
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
			if v.totalIllustrations() > 0 {
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
					_ = v.reader.SaveProgress(&reader.ReadingProgress{
						BookID:     v.bookID,
						ChapterID:  v.chapterID,
						LineIndex:  v.viewport.YOffset,
						LastReadAt: time.Now().Unix(),
					})
					return v, v.OpenChapter(v.bookID, prevID)
				}
			}
			return v, nil

		case "]", "n":
			if v.reader != nil {
				if nextID, _, ok := v.reader.NextChapter(); ok {
					_ = v.reader.SaveProgress(&reader.ReadingProgress{
						BookID:     v.bookID,
						ChapterID:  v.chapterID,
						LineIndex:  v.viewport.YOffset,
						LastReadAt: time.Now().Unix(),
					})
					return v, v.OpenChapter(v.bookID, nextID)
				}
			}
			return v, nil

		case "esc":
			// Save progress on exit
			if v.reader != nil {
				_ = v.reader.SaveProgress(&reader.ReadingProgress{
					BookID:     v.bookID,
					ChapterID:  v.chapterID,
					LineIndex:  v.viewport.YOffset,
					LastReadAt: time.Now().Unix(),
				})
			}
			return v, func() tea.Msg {
				return common.SwitchViewMsg{
					Target: common.ViewCatalog,
					BookID: v.bookID,
				}
			}
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
		rightElements = append(rightElements, lipgloss.NewStyle().
			Bold(true).
			Foreground(curTheme.Accent).
			Render(fmt.Sprintf("[插图:%d张][i]", illuCount)))
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
