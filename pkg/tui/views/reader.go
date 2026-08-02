package views

import (
	"context"
	"fmt"
	stdimage "image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	termimage "lnr-core/pkg/image"
	"lnr-core/pkg/reader"
	"lnr-core/pkg/source"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/text"
	"lnr-core/pkg/tui/common"
	"lnr-core/pkg/tui/theme"
)

type chapterContentLoadedMsg struct {
	err error
}

type illustrationLoadedMsg struct {
	index    int
	path     string
	width    int
	height   int
	fileSize int64
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

// OpenChapter loads and displays a chapter in viewport.
func (v *ReaderView) OpenChapter(bookID, chapterID string) tea.Cmd {
	v.bookID = bookID
	v.chapterID = chapterID
	v.loading = true
	v.err = nil
	v.showImageModal = false
	v.imageIndex = 0

	return func() tea.Msg {
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

// loadImageCmd fetches illustration metadata and ensures it is downloaded.
func (v *ReaderView) loadImageCmd(index int) tea.Cmd {
	return func() tea.Msg {
		if v.reader == nil {
			return illustrationLoadedMsg{index: index, err: fmt.Errorf("阅读器未初始化")}
		}
		urls := v.reader.Illustrations()
		if index < 0 || index >= len(urls) {
			return illustrationLoadedMsg{index: index, err: fmt.Errorf("插图索引超出范围")}
		}

		imgURL := urls[index]
		localPath := v.reader.IllustrationPath(imgURL)

		// Download if missing
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			if err := termimage.DownloadImageToFile(ctx, imgURL, localPath); err != nil {
				return illustrationLoadedMsg{index: index, err: fmt.Errorf("下载插图失败: %w", err)}
			}
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

		return illustrationLoadedMsg{
			index:    index,
			path:     localPath,
			width:    imgW,
			height:   imgH,
			fileSize: fileSize,
		}
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
		if v.showImageModal && msg.index == v.imageIndex {
			v.imageLoading = false
			v.imageErr = msg.err
			v.imagePath = msg.path
			v.imageWidth = msg.width
			v.imageHeight = msg.height
			v.imageSize = msg.fileSize
		}
		return v, nil

	case tea.KeyMsg:
		// Modal interaction when viewing illustration
		if v.showImageModal {
			switch msg.String() {
			case "esc", "i", "I", "q":
				v.showImageModal = false
				return v, nil
			case "left", "h", "[", "p", "up", "k":
				if v.reader != nil && v.imageIndex > 0 {
					v.imageIndex--
					v.imageLoading = true
					v.imageErr = nil
					return v, v.loadImageCmd(v.imageIndex)
				}
				return v, nil
			case "right", "l", "]", "n", "down", "j":
				if v.reader != nil && v.imageIndex < len(v.reader.Illustrations())-1 {
					v.imageIndex++
					v.imageLoading = true
					v.imageErr = nil
					return v, v.loadImageCmd(v.imageIndex)
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
			if v.reader != nil && len(v.reader.Illustrations()) > 0 {
				v.showImageModal = true
				v.imageIndex = 0
				v.imageLoading = true
				v.imageErr = nil
				return v, v.loadImageCmd(v.imageIndex)
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
	modalW := v.width - 6
	if modalW > 74 {
		modalW = 74
	}
	if modalW < 36 {
		modalW = 36
	}

	totalImages := 0
	if v.reader != nil {
		totalImages = len(v.reader.Illustrations())
	}

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
		hintStyle.Render("• [Enter/o] 高清预览 • [←/→] 翻页 • [Esc/i] 关闭"),
	)
	headerTrunc := theme.TruncateANSI(header, modalW-2, "...")
	divider := lipgloss.NewStyle().Foreground(theme.PrimaryColor).Render(strings.Repeat("─", modalW-2))

	var body string
	if v.imageLoading {
		body = lipgloss.NewStyle().
			Foreground(theme.TextMuted).
			Padding(1, 2).
			Render("[加载中] 正在拉取插图数据，请稍候...")
	} else if v.imageErr != nil {
		body = lipgloss.NewStyle().
			Foreground(theme.AccentRose).
			Padding(1, 2).
			Render(fmt.Sprintf("[错误] 插图加载失败: %v\n按 [Esc] 返回阅读", v.imageErr))
	} else {
		var specStr string
		if v.imageWidth > 0 && v.imageHeight > 0 {
			specStr = fmt.Sprintf("%d × %d 像素 (原生高清插画)", v.imageWidth, v.imageHeight)
		} else {
			specStr = "高清插画文件"
		}

		sizeStr := "已下载"
		if v.imageSize > 0 {
			if v.imageSize >= 1024*1024 {
				sizeStr = fmt.Sprintf("%.2f MB", float64(v.imageSize)/(1024*1024))
			} else {
				sizeStr = fmt.Sprintf("%.1f KB", float64(v.imageSize)/1024)
			}
		}

		labelStyle := lipgloss.NewStyle().Foreground(theme.PrimaryLight).Bold(true)
		valueStyle := lipgloss.NewStyle().Foreground(theme.TextWhite)
		statusStyle := lipgloss.NewStyle().Foreground(theme.AccentEmerald).Bold(true)

		infoLines := []string{
			fmt.Sprintf("  %s %s", labelStyle.Render("插图序号:"), valueStyle.Render(fmt.Sprintf("第 %d / %d 张插图", v.imageIndex+1, totalImages))),
			fmt.Sprintf("  %s %s", labelStyle.Render("图片规格:"), valueStyle.Render(specStr)),
			fmt.Sprintf("  %s %s", labelStyle.Render("文件大小:"), valueStyle.Render(sizeStr)),
			fmt.Sprintf("  %s %s", labelStyle.Render("本地缓存:"), statusStyle.Render("已就绪 (按回车即刻秒级调出系统预览)")),
			"",
			lipgloss.NewStyle().Foreground(theme.BorderColor).Render("  " + strings.Repeat("┄", modalW-6)),
			"",
			lipgloss.NewStyle().Foreground(theme.AccentAmber).Bold(true).Render("  操作指南:"),
			lipgloss.NewStyle().Foreground(theme.TextMuted).Render("  • 按 [Enter] / [Space] / [o] 立即调出系统原生高清预览"),
			lipgloss.NewStyle().Foreground(theme.TextDim).Render("    (macOS 原生 Quick Look 秒级弹出，Retina 缩放与全屏)"),
			lipgloss.NewStyle().Foreground(theme.TextMuted).Render("  • 按 [← / →] 或 [h / l] 切换上一张 / 下一张插图"),
			lipgloss.NewStyle().Foreground(theme.TextMuted).Render("  • 按 [Esc] 或 [i] 关闭插图查看器，返回沉浸阅读"),
		}
		body = strings.Join(infoLines, "\n")
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
