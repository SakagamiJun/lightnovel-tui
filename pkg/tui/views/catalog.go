package views

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/SakagamiJun/lightnovel-tui/pkg/epub"
	"github.com/SakagamiJun/lightnovel-tui/pkg/model"
	"github.com/SakagamiJun/lightnovel-tui/pkg/source"
	"github.com/SakagamiJun/lightnovel-tui/pkg/storage"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/common"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/theme"
)

type catalogResultMsg struct {
	detail  *model.BookDetail
	catalog *model.BookCatalog
	err     error
}

// CatalogView renders the volume and chapter tree of a novel.
type CatalogView struct {
	store               *storage.Storage
	src                 source.DataSource
	bookID              string
	detail              *model.BookDetail
	catalog             *model.BookCatalog
	cursor              int
	offset              int
	flatItems           []flatChapterItem
	loading             bool
	err                 error
	width               int
	height              int
	selectedVols        map[int]bool // 1-indexed volume index -> true
	showExportModal     bool
	exportFocusRow      int  // 0: 范围, 1: 插图, 2: 模式
	exportScopeSelected bool // true: 所选分卷 (共 N 卷), false: 全部书籍分卷
	exportIncludeImages bool // true: 包含插图, false: 纯文本轻量 (无图)
	exportSplitVolumes  bool // true: 分卷独立文件, false: 合并为单本

	onSelect            func(bookID, chapterID string) tea.Cmd
	onDownload          func(bookID string, volumeIndex int) tea.Cmd
	onExport            func(bookID string, volumeIndex int) tea.Cmd
	onExportWithOptions func(bookID string, opt epub.ExportOption) tea.Cmd
	onBack              func() tea.Cmd
}

type flatChapterItem struct {
	isVolume bool
	volIndex int
	volTitle string
	chapID   string
	title    string
}

// NewCatalogView constructs a catalog navigation view.
func NewCatalogView(store *storage.Storage, src source.DataSource, onSelect func(bookID, chapterID string) tea.Cmd) *CatalogView {
	return &CatalogView{
		store:               store,
		src:                 src,
		flatItems:           make([]flatChapterItem, 0),
		selectedVols:        make(map[int]bool),
		exportIncludeImages: true,
		onSelect:            onSelect,
	}
}

// SetOnBack registers back navigation callback.
func (v *CatalogView) SetOnBack(fn func() tea.Cmd) {
	v.onBack = fn
}

// SetOnDownload registers download callback.
func (v *CatalogView) SetOnDownload(fn func(bookID string, volumeIndex int) tea.Cmd) {
	v.onDownload = fn
}

// SetOnExport registers export callback.
func (v *CatalogView) SetOnExport(fn func(bookID string, volumeIndex int) tea.Cmd) {
	v.onExport = fn
}

// SetOnExportWithOptions registers export with options callback.
func (v *CatalogView) SetOnExportWithOptions(fn func(bookID string, opt epub.ExportOption) tea.Cmd) {
	v.onExportWithOptions = fn
}

// IsExportModalOpen returns whether export modal dialog is active.
func (v *CatalogView) IsExportModalOpen() bool {
	return v.showExportModal
}

// CloseExportModal closes the export modal dialog.
func (v *CatalogView) CloseExportModal() {
	v.showExportModal = false
}

// LoadBook triggers fetching catalog for a given book ID.
func (v *CatalogView) LoadBook(bookID string) tea.Cmd {
	v.bookID = bookID
	v.loading = true
	v.err = nil
	v.cursor = 0
	v.offset = 0

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		detail, err := v.store.LoadBookDetail(bookID)
		if err != nil {
			detail, err = v.src.GetBookDetail(ctx, bookID)
			if err == nil {
				_ = v.store.SaveBookDetail(detail)
			}
		}

		catalog, err := v.store.LoadCatalog(bookID)
		if err != nil {
			catalog, err = v.src.GetCatalog(ctx, bookID)
			if err == nil {
				_ = v.store.SaveCatalog(catalog)
			}
		}

		return catalogResultMsg{
			detail:  detail,
			catalog: catalog,
			err:     err,
		}
	}
}

func (v *CatalogView) Init() tea.Cmd {
	return nil
}

func (v *CatalogView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

func (v *CatalogView) visibleLines() int {
	// Total available height minus top title (1 line) and indicators (2 lines) = 3 lines
	lines := v.height - 3
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (v *CatalogView) adjustOffset() {
	visible := v.visibleLines()
	if v.cursor < v.offset {
		v.offset = v.cursor
	} else if v.cursor >= v.offset+visible {
		v.offset = v.cursor - visible + 1
	}
	// Safety bound check
	if v.offset < 0 {
		v.offset = 0
	}
	maxOffset := len(v.flatItems) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if v.offset > maxOffset {
		v.offset = maxOffset
	}
}

func (v *CatalogView) countSelectedVols() int {
	if v.selectedVols == nil {
		return 0
	}
	cnt := 0
	for _, sel := range v.selectedVols {
		if sel {
			cnt++
		}
	}
	return cnt
}

func (v *CatalogView) renderExportModal() string {
	maxWidth := v.width - 6
	if maxWidth > 62 {
		maxWidth = 62
	}
	if maxWidth < 36 {
		maxWidth = 36
	}

	selectedCount := v.countSelectedVols()
	totalVols := 0
	if v.catalog != nil {
		totalVols = len(v.catalog.Volumes)
	}

	var sb strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
		Render("[EPUB 导出选项设置]")
	sb.WriteString(title + "\n\n")

	// Row 0: 范围
	var opt0Scope string
	if selectedCount > 0 {
		var r1, r2 string
		if v.exportScopeSelected {
			r1 = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentSky).Render(fmt.Sprintf("[● 所选分卷 (%d卷)]", selectedCount))
			r2 = lipgloss.NewStyle().Foreground(theme.TextDim).Render(fmt.Sprintf("(○ 全部书籍 (%d卷))", totalVols))
		} else {
			r1 = lipgloss.NewStyle().Foreground(theme.TextDim).Render(fmt.Sprintf("(○ 所选分卷 (%d卷))", selectedCount))
			r2 = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentSky).Render(fmt.Sprintf("[● 全部书籍 (%d卷)]", totalVols))
		}
		opt0Scope = r1 + "  " + r2
	} else {
		opt0Scope = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentSky).Render(fmt.Sprintf("[● 全部书籍分卷 (%d卷)]", totalVols))
	}

	// Row 1: 插图
	var opt1Images string
	if v.exportIncludeImages {
		r1 := lipgloss.NewStyle().Bold(true).Foreground(theme.AccentEmerald).Render("[● 包含插图 (全彩原图)]")
		r2 := lipgloss.NewStyle().Foreground(theme.TextDim).Render("(○ 纯文本轻量 (无图))")
		opt1Images = r1 + "  " + r2
	} else {
		r1 := lipgloss.NewStyle().Foreground(theme.TextDim).Render("(○ 包含插图 (全彩原图))")
		r2 := lipgloss.NewStyle().Bold(true).Foreground(theme.AccentAmber).Render("[● 纯文本轻量 (无图)]")
		opt1Images = r1 + "  " + r2
	}

	// Row 2: 模式
	var opt2Mode string
	if !v.exportSplitVolumes {
		r1 := lipgloss.NewStyle().Bold(true).Foreground(theme.AccentSky).Render("[● 合并为单本 EPUB]")
		r2 := lipgloss.NewStyle().Foreground(theme.TextDim).Render("(○ 每卷独立文件)")
		opt2Mode = r1 + "  " + r2
	} else {
		r1 := lipgloss.NewStyle().Foreground(theme.TextDim).Render("(○ 合并为单本 EPUB)")
		r2 := lipgloss.NewStyle().Bold(true).Foreground(theme.AccentSky).Render("[● 每卷独立文件]")
		opt2Mode = r1 + "  " + r2
	}

	rows := []struct {
		label   string
		content string
	}{
		{"导出范围: ", opt0Scope},
		{"包含插图: ", opt1Images},
		{"导出模式: ", opt2Mode},
	}

	for i, r := range rows {
		isFocused := i == v.exportFocusRow
		prefix := "    │ "
		labelStyle := lipgloss.NewStyle().Foreground(theme.TextMuted)
		if isFocused {
			prefix = "  ▎ ▶ "
			labelStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentSky)
		}
		sb.WriteString(prefix + labelStyle.Render(r.label) + r.content + "\n")
	}

	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(theme.BorderColor).Render(strings.Repeat("─", maxWidth-4)) + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.TextMuted).
		Render("[Tab/↑/↓] 移动选项  •  [←/→/Space] 切换  •  [Enter] 开始导出  •  [Esc] 取消"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.PrimaryColor).
		Padding(1, 2).
		Width(maxWidth).
		Render(sb.String())
}

func (v *CatalogView) Update(msg tea.Msg) (*CatalogView, tea.Cmd) {
	if v.selectedVols == nil {
		v.selectedVols = make(map[int]bool)
	}

	switch msg := msg.(type) {
	case catalogResultMsg:
		v.loading = false
		v.detail = msg.detail
		v.catalog = msg.catalog
		v.err = msg.err
		v.cursor = 0
		v.offset = 0
		v.selectedVols = make(map[int]bool)
		v.showExportModal = false
		v.flattenItems()
		return v, nil

	case tea.MouseMsg:
		if v.showExportModal {
			return v, nil
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if v.cursor > 0 {
				v.cursor--
				v.adjustOffset()
			}
		case tea.MouseButtonWheelDown:
			if v.cursor < len(v.flatItems)-1 {
				v.cursor++
				v.adjustOffset()
			}
		}

	case tea.KeyMsg:
		if v.showExportModal {
			switch msg.String() {
			case "esc":
				v.showExportModal = false
				return v, nil
			case "up", "k":
				if v.exportFocusRow > 0 {
					v.exportFocusRow--
				} else {
					v.exportFocusRow = 2
				}
				return v, nil
			case "down", "j", "tab":
				v.exportFocusRow = (v.exportFocusRow + 1) % 3
				return v, nil
			case "shift+tab":
				if v.exportFocusRow > 0 {
					v.exportFocusRow--
				} else {
					v.exportFocusRow = 2
				}
				return v, nil
			case "left", "h", "right", "l", " ":
				selectedCount := v.countSelectedVols()
				switch v.exportFocusRow {
				case 0:
					if selectedCount > 0 {
						v.exportScopeSelected = !v.exportScopeSelected
					}
				case 1:
					v.exportIncludeImages = !v.exportIncludeImages
				case 2:
					v.exportSplitVolumes = !v.exportSplitVolumes
				}
				return v, nil
			case "enter":
				v.showExportModal = false
				selectedCount := v.countSelectedVols()
				var volIndices []int
				if v.exportScopeSelected && selectedCount > 0 {
					for volIdx, sel := range v.selectedVols {
						if sel {
							volIndices = append(volIndices, volIdx)
						}
					}
					sort.Ints(volIndices)
				}
				opt := epub.ExportOption{
					VolumeIndexes: volIndices,
					NoImages:      !v.exportIncludeImages,
					SplitVolumes:  v.exportSplitVolumes,
				}
				if v.onExportWithOptions != nil {
					return v, v.onExportWithOptions(v.bookID, opt)
				} else if v.onExport != nil {
					if opt.SplitVolumes {
						return v, v.onExport(v.bookID, common.ExportModeAllVolumesSeparate)
					} else if len(volIndices) == 1 {
						return v, v.onExport(v.bookID, volIndices[0])
					} else {
						return v, v.onExport(v.bookID, common.ExportModeFullBook)
					}
				}
				return v, nil
			}
			return v, nil
		}

		visible := v.visibleLines()
		switch msg.String() {
		case "up", "k":
			if v.cursor > 0 {
				v.cursor--
				v.adjustOffset()
			}
		case "down", "j":
			if v.cursor < len(v.flatItems)-1 {
				v.cursor++
				v.adjustOffset()
			}
		case "pgup", "ctrl+u", "b":
			v.cursor -= visible
			if v.cursor < 0 {
				v.cursor = 0
			}
			v.adjustOffset()
		case "pgdown", "ctrl+d", "f":
			v.cursor += visible
			if v.cursor >= len(v.flatItems) {
				v.cursor = len(v.flatItems) - 1
			}
			v.adjustOffset()
		case "g", "home":
			v.cursor = 0
			v.offset = 0
		case "G", "end":
			if len(v.flatItems) > 0 {
				v.cursor = len(v.flatItems) - 1
				v.adjustOffset()
			}
		case " ":
			if len(v.flatItems) > 0 && v.cursor < len(v.flatItems) {
				item := v.flatItems[v.cursor]
				v.selectedVols[item.volIndex] = !v.selectedVols[item.volIndex]
				if v.countSelectedVols() > 0 {
					v.exportScopeSelected = true
				} else {
					v.exportScopeSelected = false
				}
			}
		case "enter":
			if len(v.flatItems) > 0 && v.cursor < len(v.flatItems) {
				item := v.flatItems[v.cursor]
				if !item.isVolume {
					if v.onSelect != nil {
						return v, v.onSelect(v.bookID, item.chapID)
					}
					return v, func() tea.Msg {
						return common.SwitchViewMsg{
							Target: common.ViewReader,
							BookID: v.bookID,
							ChapID: item.chapID,
						}
					}
				}
			}
		case "d", "c":
			if len(v.flatItems) > 0 && v.cursor < len(v.flatItems) {
				item := v.flatItems[v.cursor]
				if item.isVolume && v.onDownload != nil {
					return v, v.onDownload(v.bookID, item.volIndex)
				}
			}
		case "e":
			if len(v.flatItems) > 0 && v.cursor < len(v.flatItems) {
				item := v.flatItems[v.cursor]
				if item.isVolume && v.onExport != nil {
					return v, v.onExport(v.bookID, item.volIndex)
				}
			}
		case "D", "C":
			if v.onDownload != nil {
				return v, v.onDownload(v.bookID, 0)
			}
		case "E":
			if v.countSelectedVols() > 0 {
				v.exportScopeSelected = true
			} else {
				v.exportScopeSelected = false
			}
			v.showExportModal = true
			v.exportFocusRow = 0
			return v, nil
		case "s", "S":
			if v.onExport != nil {
				return v, v.onExport(v.bookID, common.ExportModeAllVolumesSeparate)
			}
		case "esc":
			if v.onBack != nil {
				return v, v.onBack()
			}
			return v, func() tea.Msg {
				return common.SwitchViewMsg{
					Target: common.ViewBookshelf,
				}
			}
		}
	}
	return v, nil
}

func (v *CatalogView) flattenItems() {
	v.flatItems = make([]flatChapterItem, 0)
	if v.catalog == nil {
		return
	}
	for vi, vol := range v.catalog.Volumes {
		v.flatItems = append(v.flatItems, flatChapterItem{
			isVolume: true,
			volIndex: vi + 1,
			volTitle: vol.Title,
			title:    vol.Title,
		})
		for _, ch := range vol.Chapters {
			v.flatItems = append(v.flatItems, flatChapterItem{
				isVolume: false,
				volIndex: vi + 1,
				volTitle: vol.Title,
				chapID:   ch.ID,
				title:    ch.Title,
			})
		}
	}
}

func (v *CatalogView) View() string {
	if v.loading {
		return "\n[加载中] 正在获取小说分卷目录，请稍候..."
	}
	if v.err != nil {
		return fmt.Sprintf("\n[错误] 获取目录失败: %v\n按 [Esc] 返回书架", v.err)
	}

	if len(v.flatItems) == 0 {
		return "\n暂无目录内容。按 [Esc] 返回书架"
	}

	if v.showExportModal {
		return v.renderExportModal()
	}

	var sb strings.Builder
	maxWidth := v.width - 2
	if maxWidth < 30 {
		maxWidth = 30
	}

	if v.detail != nil {
		curPos := v.cursor + 1
		total := len(v.flatItems)
		var selInfo string
		selCount := v.countSelectedVols()
		if selCount > 0 {
			selInfo = fmt.Sprintf("  •  已勾选 %d 卷", selCount)
		}
		var headerText string
		if maxWidth >= 90 {
			headerText = fmt.Sprintf(" %s%s  •  [%d/%d 项]  •  [Enter] 阅读  •  [Space] 勾选分卷  •  [E] 导出选项",
				v.detail.Title, selInfo, curPos, total)
		} else {
			headerText = fmt.Sprintf(" %s%s  •  [%d/%d]  •  [Enter]阅读  •  [Space]勾选  •  [E]导出",
				v.detail.Title, selInfo, curPos, total)
		}
		header := lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryLight).
			Render(theme.TruncateANSI(headerText, maxWidth, "..."))
		sb.WriteString(header + "\n")
	}

	v.adjustOffset()
	visible := v.visibleLines()
	start := v.offset
	end := start + visible
	if end > len(v.flatItems) {
		end = len(v.flatItems)
	}

	// Top indicator if truncated
	if start > 0 {
		msg := fmt.Sprintf("  ▲ 上方还有 %d 项已折叠 (按 [g] 到顶部) ", start)
		ruleLen := maxWidth - theme.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentAmber).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.BorderColor).
			Render(strings.Repeat("─", maxWidth)) + "\n")
	}

	// Windowed slice rendering
	for i := start; i < end; i++ {
		item := v.flatItems[i]
		isSelected := i == v.cursor
		if item.isVolume {
			var checkBadge string
			if v.selectedVols[item.volIndex] {
				checkBadge = lipgloss.NewStyle().Bold(true).Foreground(theme.TextWhite).
					Background(theme.SecondaryColor).Padding(0, 1).Render("[x]")
			} else {
				checkBadge = lipgloss.NewStyle().Foreground(theme.TextMuted).Render("[ ]")
			}
			volBadge := theme.BadgeInfo.Render("分卷")
			volText := fmt.Sprintf(" %s [分卷] %s %s ", checkBadge, item.volTitle, volBadge)
			ruleLen := maxWidth - theme.StringWidth(volText)
			if ruleLen < 0 {
				ruleLen = 0
			}
			volLine := volText + strings.Repeat("─", ruleLen)
			volLineTrunc := theme.TruncateANSI(volLine, maxWidth, "...")
			sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(theme.AccentSky).
				Render(volLineTrunc))
			sb.WriteString("\n")
		} else {
			if isSelected {
				prefix := "  ▎ ▶ "
				line := theme.TruncateANSI(prefix+item.title, maxWidth, "...")
				sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(theme.TextWhite).
					Background(theme.HighlightBg).Width(maxWidth).Render(line))
				sb.WriteString("\n")
			} else {
				prefix := "    │ "
				line := theme.TruncateANSI(prefix+item.title, maxWidth, "...")
				sb.WriteString(lipgloss.NewStyle().Foreground(theme.TextWhite).
					Width(maxWidth).Render(line))
				sb.WriteString("\n")
			}
		}
	}

	// Bottom indicator if truncated
	if end < len(v.flatItems) {
		remaining := len(v.flatItems) - end
		msg := fmt.Sprintf("  ▼ 下方还有 %d 项已折叠 (按 [G] 跳到底部) ", remaining)
		ruleLen := maxWidth - runewidth.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentAmber).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	} else {
		msg := "  [全部] 已显示全部分卷与章节 "
		ruleLen := maxWidth - runewidth.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentEmerald).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	}

	return sb.String()
}
