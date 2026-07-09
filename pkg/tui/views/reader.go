package views

import (
	"context"
	"fmt"
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
	rendered string
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
	imageErr       error
	imageViewport  viewport.Model

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

	if v.showImageModal {
		modalW := width - 6
		if modalW < 20 {
			modalW = 20
		}
		modalH := height - 6
		if modalH < 10 {
			modalH = 10
		}
		v.imageViewport.Width = modalW
		v.imageViewport.Height = modalH - 3
	}
}

// loadImageCmd fetches and renders the illustration at specified index.
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

		// Calculate available modal viewport dimensions
		renderW := v.width - 10
		if renderW < 20 {
			renderW = 20
		}
		// Unclamp vertical height: viewport allows vertical scrolling with j/k/arrows,
		// so allocating up to 120 character rows (240 vertical half-block pixels) produces
		// 8x higher resolution than squishing down to 14 terminal lines.
		renderH := 120
		if v.height*2 > renderH {
			renderH = v.height * 2
		}

		// Render with auto terminal protocol (iTerm2, Kitty, or 24-bit TrueColor half-block)
		rendered, err := termimage.RenderAutoFile(localPath, renderW, renderH)
		if err != nil {
			return illustrationLoadedMsg{index: index, err: fmt.Errorf("渲染插图失败: %w", err)}
		}

		return illustrationLoadedMsg{
			index:    index,
			path:     localPath,
			rendered: rendered,
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
			if msg.err == nil {
				v.imageViewport.SetContent(msg.rendered)
				v.imageViewport.GotoTop()
			}
		}
		return v, nil

	case tea.KeyMsg:
		// Modal interaction when viewing illustration
		if v.showImageModal {
			switch msg.String() {
			case "esc", "i", "I":
				v.showImageModal = false
				return v, nil
			case "left", "h", "[", "p":
				if v.reader != nil && v.imageIndex > 0 {
					v.imageIndex--
					v.imageLoading = true
					v.imageErr = nil
					return v, v.loadImageCmd(v.imageIndex)
				}
				return v, nil
			case "right", "l", "]", "n":
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
				var ivCmd tea.Cmd
				v.imageViewport, ivCmd = v.imageViewport.Update(msg)
				return v, ivCmd
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

				modalW := v.width - 6
				if modalW < 20 {
					modalW = 20
				}
				modalH := v.height - 6
				if modalH < 10 {
					modalH = 10
				}
				v.imageViewport = viewport.New(modalW, modalH-3)
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
	if modalW < 20 {
		modalW = 20
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
		hintStyle.Render("• [Enter/o] 高清预览(QuickLook) • [←/→] 翻页 • [j/k] 滚动 • [Esc/i] 关闭"),
	)

	var body string
	if v.imageLoading {
		body = lipgloss.NewStyle().
			Foreground(theme.TextMuted).
			Padding(2, 4).
			Render("[加载中] 正在拉取并渲染插图，请稍候...")
	} else if v.imageErr != nil {
		body = lipgloss.NewStyle().
			Foreground(theme.AccentRose).
			Padding(2, 4).
			Render(fmt.Sprintf("[错误] 插图加载失败: %v\n按 [Esc] 返回阅读", v.imageErr))
	} else {
		body = v.imageViewport.View()
	}

	divider := lipgloss.NewStyle().Foreground(theme.PrimaryColor).Render(strings.Repeat("─", modalW))
	modalContent := header + "\n" + divider + "\n" + body

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.PrimaryColor).
		Width(modalW).
		Render(modalContent)

	return box
}
