package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"lnr-core/pkg/model"
	"lnr-core/pkg/source"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/common"
	"lnr-core/pkg/tui/theme"
)

type updateCheckResultMsg struct {
	updates map[string]int
	err     error
}

// BookshelfView displays locally cached novels with cursor navigation.
type BookshelfView struct {
	store           *storage.Storage
	src             source.DataSource
	books           []model.BookDetail
	pinnedSet       map[string]bool
	cursor          int
	offset          int
	width           int
	height          int
	loaded          bool
	confirmDelete   bool
	sortCriteria    storage.SortCriteria
	checkingUpdates bool
	updatesMap      map[string]int
	onSelect        func(bookID string) tea.Cmd
	onExport        func(bookID string, volumeIndex int) tea.Cmd
}

// NewBookshelfView constructs bookshelf model.
func NewBookshelfView(store *storage.Storage, onSelect func(bookID string) tea.Cmd) *BookshelfView {
	return &BookshelfView{
		store:        store,
		books:        make([]model.BookDetail, 0),
		pinnedSet:    make(map[string]bool),
		updatesMap:   make(map[string]int),
		sortCriteria: storage.SortByLastRead,
		onSelect:     onSelect,
	}
}

// SetSource provides data source for checking online chapter updates.
func (v *BookshelfView) SetSource(src source.DataSource) {
	v.src = src
}

// SetOnExport registers export callback.
func (v *BookshelfView) SetOnExport(fn func(bookID string, volumeIndex int) tea.Cmd) {
	v.onExport = fn
}

// ReloadLoads cached books from storage and partitions pinned books to the top.
func (v *BookshelfView) Reload() {
	if v.store != nil {
		books, err := v.store.ListCachedBooks()
		if err == nil {
			pinnedIDs, _ := v.store.GetPinnedBookIDs()
			v.pinnedSet = make(map[string]bool, len(pinnedIDs))
			for _, id := range pinnedIDs {
				v.pinnedSet[id] = true
			}

			// Sort books using active sort criteria and pinned status
			v.store.SortBooks(books, v.sortCriteria)
			v.books = books
		}
	}
	if v.pinnedSet == nil {
		v.pinnedSet = make(map[string]bool)
	}
	if v.cursor >= len(v.books) && len(v.books) > 0 {
		v.cursor = len(v.books) - 1
	}
	v.loaded = true
}

// SetBooks sets cached books directly (for testing and external feeds).
func (v *BookshelfView) SetBooks(books []model.BookDetail) {
	v.books = books
	if v.pinnedSet == nil {
		v.pinnedSet = make(map[string]bool)
	}
	v.loaded = true
	v.cursor = 0
	v.offset = 0
}

func (v *BookshelfView) Init() tea.Cmd {
	return func() tea.Msg {
		v.Reload()
		return nil
	}
}

func (v *BookshelfView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

func (v *BookshelfView) visibleCards() int {
	// Fixed lines: TitleBar (1) + TopInd (1) + BotInd (1) = 3 lines.
	// Each modern card takes exactly 3 lines.
	avail := v.height - 3
	if avail < 3 {
		return 1
	}
	cards := avail / 3
	if cards < 1 {
		cards = 1
	}
	return cards
}

func (v *BookshelfView) adjustOffset() {
	visible := v.visibleCards()
	if v.cursor < v.offset {
		v.offset = v.cursor
	} else if v.cursor >= v.offset+visible {
		v.offset = v.cursor - visible + 1
	}
	if v.offset < 0 {
		v.offset = 0
	}
	maxOffset := len(v.books) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if v.offset > maxOffset {
		v.offset = maxOffset
	}
}

func (v *BookshelfView) checkUpdatesCmd() tea.Cmd {
	v.checkingUpdates = true
	return func() tea.Msg {
		if v.src == nil || v.store == nil {
			return updateCheckResultMsg{err: fmt.Errorf("数据源未就绪")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()

		updates := make(map[string]int)
		for _, b := range v.books {
			info, err := v.store.CheckBookUpdate(ctx, v.src, b.ID)
			if err == nil && info.HasUpdate {
				updates[b.ID] = info.NewChapterCount
			}
		}
		return updateCheckResultMsg{updates: updates}
	}
}

func (v *BookshelfView) Update(msg tea.Msg) (*BookshelfView, tea.Cmd) {
	switch msg := msg.(type) {
	case updateCheckResultMsg:
		v.checkingUpdates = false
		v.updatesMap = msg.updates
		count := len(msg.updates)
		var statusText string
		if count > 0 {
			statusText = fmt.Sprintf("检查更新完毕: 发现 %d 部小说有新章节！", count)
		} else {
			statusText = "全书架藏书均为最新章节，暂无更新"
		}
		return v, func() tea.Msg {
			return common.StatusMsg(statusText)
		}

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if v.cursor > 0 {
				v.cursor--
				v.adjustOffset()
			}
		case tea.MouseButtonWheelDown:
			if v.cursor < len(v.books)-1 {
				v.cursor++
				v.adjustOffset()
			}
		}

	case tea.KeyMsg:
		if v.confirmDelete {
			switch msg.String() {
			case "y", "Y":
				if len(v.books) > 0 && v.cursor < len(v.books) {
					b := v.books[v.cursor]
					if v.store != nil {
						_ = v.store.DeleteBook(b.ID)
					}
					v.confirmDelete = false
					v.Reload()
					return v, func() tea.Msg {
						return common.StatusMsg(fmt.Sprintf("已删除《%s》本地缓存及章节", b.Title))
					}
				}
				v.confirmDelete = false
			case "n", "N", "esc":
				v.confirmDelete = false
				return v, func() tea.Msg {
					return common.StatusMsg("已取消删除操作")
				}
			}
			return v, nil
		}

		visible := v.visibleCards()
		switch msg.String() {
		case "up", "k":
			if v.cursor > 0 {
				v.cursor--
				v.adjustOffset()
			}
		case "down", "j":
			if v.cursor < len(v.books)-1 {
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
			if v.cursor >= len(v.books) {
				v.cursor = len(v.books) - 1
			}
			v.adjustOffset()
		case "g", "home":
			v.cursor = 0
			v.offset = 0
		case "G", "end":
			if len(v.books) > 0 {
				v.cursor = len(v.books) - 1
				v.adjustOffset()
			}
		case "o", "O":
			switch v.sortCriteria {
			case storage.SortByLastRead:
				v.sortCriteria = storage.SortByLastUpdated
			case storage.SortByLastUpdated:
				v.sortCriteria = storage.SortByTitle
			case storage.SortByTitle:
				v.sortCriteria = storage.SortBySize
			case storage.SortBySize:
				v.sortCriteria = storage.SortByLastRead
			}
			v.Reload()
			v.cursor = 0
			v.offset = 0
			sortName := storage.SortCriteriaNames[v.sortCriteria]
			return v, func() tea.Msg {
				return common.StatusMsg(fmt.Sprintf("书架排序已切换为: %s", sortName))
			}
		case "u", "U":
			if v.src == nil {
				return v, func() tea.Msg {
					return common.StatusMsg("数据源未连接，无法检查更新")
				}
			}
			if len(v.books) == 0 {
				return v, func() tea.Msg {
					return common.StatusMsg("书架暂无藏书")
				}
			}
			return v, tea.Batch(
				func() tea.Msg {
					return common.StatusMsg("正在联网检查书架更新...")
				},
				v.checkUpdatesCmd(),
			)
		case "p":
			if len(v.books) > 0 && v.cursor < len(v.books) {
				b := v.books[v.cursor]
				if v.store != nil {
					pinnedNow, _ := v.store.TogglePinBook(b.ID)
					v.Reload()
					for idx, bk := range v.books {
						if bk.ID == b.ID {
							v.cursor = idx
							break
						}
					}
					v.adjustOffset()
					statusText := fmt.Sprintf("已置顶《%s》", b.Title)
					if !pinnedNow {
						statusText = fmt.Sprintf("已取消置顶《%s》", b.Title)
					}
					return v, func() tea.Msg {
						return common.StatusMsg(statusText)
					}
				}
			}
		case "d", "x", "delete":
			if len(v.books) > 0 && v.cursor < len(v.books) {
				v.confirmDelete = true
				return v, nil
			}
		case "e":
			if len(v.books) > 0 && v.cursor < len(v.books) && v.onExport != nil {
				return v, v.onExport(v.books[v.cursor].ID, common.ExportModeFullBook)
			}
		case "s":
			if len(v.books) > 0 && v.cursor < len(v.books) && v.onExport != nil {
				return v, v.onExport(v.books[v.cursor].ID, common.ExportModeAllVolumesSeparate)
			}
		case "enter":
			if len(v.books) > 0 && v.cursor < len(v.books) {
				selectedID := v.resultsID(v.cursor)
				if selectedID != "" {
					if v.onSelect != nil {
						return v, v.onSelect(selectedID)
					}
					return v, func() tea.Msg {
						return common.SwitchViewMsg{
							Target: common.ViewCatalog,
							BookID: selectedID,
						}
					}
				}
			}
		case "r":
			v.Reload()
		}
	}
	return v, nil
}

func (v *BookshelfView) resultsID(idx int) string {
	if idx >= 0 && idx < len(v.books) {
		return v.books[idx].ID
	}
	return ""
}

func (v *BookshelfView) View() string {
	if !v.loaded {
		v.Reload()
	}

	maxWidth := v.width - 2
	if maxWidth < 30 {
		maxWidth = 30
	}

	if len(v.books) == 0 {
		emptyBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.BorderColor).
			Padding(2, 4).
			Align(lipgloss.Center).
			Render("本地书架暂无藏书\n\n尚未缓存任何轻小说\n按 [Tab] 切换到在线检索，输入书名检索并下载阅读！")
		return emptyBox
	}

	var sb strings.Builder
	curPos := v.cursor + 1
	var titleBar string
	if v.confirmDelete && v.cursor < len(v.books) {
		delPrompt := fmt.Sprintf(" [确认删除] 确认删除《%s》本地缓存？按 [y] 确认 / 按 [n/Esc] 取消", v.books[v.cursor].Title)
		titleBar = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentRose).
			Render(runewidth.Truncate(delPrompt, maxWidth, "..."))
	} else {
		sortName := storage.SortCriteriaNames[v.sortCriteria]
		var updateStatus string
		if v.checkingUpdates {
			updateStatus = " • [检查中...]"
		}
		titleText := fmt.Sprintf(" 本地藏书库 (%d/%d 本) • [排序: %s(o)] • [u] 检查更新 • [p] 置顶 • [x] 删除%s",
			curPos, len(v.books), sortName, updateStatus)
		titleBar = lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
			Render(runewidth.Truncate(titleText, maxWidth, "..."))
	}
	sb.WriteString(titleBar + "\n")

	v.adjustOffset()
	visible := v.visibleCards()
	start := v.offset
	end := start + visible
	if end > len(v.books) {
		end = len(v.books)
	}

	if start > 0 {
		msg := fmt.Sprintf("  ▲ 上方还有 %d 本藏书已折叠 (按 [g] 到顶部) ", start)
		ruleLen := maxWidth - runewidth.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentAmber).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.BorderColor).
			Render(strings.Repeat("─", maxWidth)) + "\n")
	}

	const (
		barActive       = "▎ ▶ "
		barActiveIndent = "▎   "
		barInactive     = "│   "
	)

	for i := start; i < end; i++ {
		b := v.books[i]
		isSelected := i == v.cursor

		// Badges for Line 1: only essential status
		var pinBadge string
		if v.pinnedSet != nil && v.pinnedSet[b.ID] {
			pinBadge = theme.BadgeWarning.Render("置顶")
		}
		var updateBadge string
		if v.updatesMap != nil && v.updatesMap[b.ID] > 0 {
			updateBadge = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.TextWhite).
				Background(theme.AccentEmerald).
				Padding(0, 1).
				Render(fmt.Sprintf("更新:+%d章", v.updatesMap[b.ID]))
		}
		var compBadge string
		if b.IsComplete {
			compBadge = theme.BadgeSuccess.Render("完结")
		}

		var line1Badges string
		badges := make([]string, 0, 3)
		if pinBadge != "" {
			badges = append(badges, pinBadge)
		}
		if updateBadge != "" {
			badges = append(badges, updateBadge)
		}
		if compBadge != "" {
			badges = append(badges, compBadge)
		}
		line1Badges = strings.Join(badges, " ")

		desc := strings.ReplaceAll(b.Description, "\r", " ")
		desc = strings.ReplaceAll(desc, "\n", " ")
		desc = strings.TrimSpace(desc)
		if desc == "" {
			desc = "已缓存到本地，按 [Enter] 查看分卷目录"
		}

		wordCountStr := theme.FormatWordCount(b.WordCount)
		pubStr := b.Publisher
		if pubStr == "" {
			pubStr = "未知文库"
		}
		statusStr := "连载中"
		if b.IsComplete {
			statusStr = "已完结"
		}

		if isSelected {
			// Line 1: Full Book Title + Minimal Badges
			prefix := barActive
			prefixWidth := lipgloss.Width(prefix)
			badgesWidth := lipgloss.Width(line1Badges)
			extraGap := 0
			if line1Badges != "" {
				extraGap = 2
			}
			titleBudget := maxWidth - prefixWidth - badgesWidth - extraGap
			if titleBudget < 10 {
				titleBudget = 10
			}
			titleTrunc := b.Title
			if runewidth.StringWidth(b.Title) > titleBudget {
				titleTrunc = runewidth.Truncate(b.Title, titleBudget, "...")
			}
			var line1Content string
			if line1Badges != "" {
				line1Content = fmt.Sprintf("%s%s  %s", prefix,
					lipgloss.NewStyle().Bold(true).Foreground(theme.TextWhite).Render(titleTrunc),
					line1Badges)
			} else {
				line1Content = fmt.Sprintf("%s%s", prefix,
					lipgloss.NewStyle().Bold(true).Foreground(theme.TextWhite).Render(titleTrunc))
			}
			line1 := lipgloss.NewStyle().Background(theme.HighlightBg).Width(maxWidth).Render(line1Content)

			// Line 2: Rich Meta Info
			metaContent := fmt.Sprintf("%s作者: %s    文库: %s    字数: %s    状态: %s    ID: #%s",
				barActiveIndent, b.Author, pubStr, wordCountStr, statusStr, b.ID)
			line2Trunc := runewidth.Truncate(metaContent, maxWidth, "...")
			line2 := lipgloss.NewStyle().Foreground(theme.PrimaryLight).Background(theme.HighlightBg).Width(maxWidth).Render(line2Trunc)

			// Line 3: Description Preview
			descContent := fmt.Sprintf("%s简介: %s", barActiveIndent, desc)
			descTrunc := runewidth.Truncate(descContent, maxWidth, "...")
			line3 := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")).Background(theme.HighlightBg).Width(maxWidth).Render(descTrunc)

			sb.WriteString(line1 + "\n")
			sb.WriteString(line2 + "\n")
			sb.WriteString(line3 + "\n")
		} else {
			// Line 1: Full Book Title + Minimal Badges
			prefix := barInactive
			prefixWidth := lipgloss.Width(prefix)
			badgesWidth := lipgloss.Width(line1Badges)
			extraGap := 0
			if line1Badges != "" {
				extraGap = 2
			}
			titleBudget := maxWidth - prefixWidth - badgesWidth - extraGap
			if titleBudget < 10 {
				titleBudget = 10
			}
			titleTrunc := b.Title
			if runewidth.StringWidth(b.Title) > titleBudget {
				titleTrunc = runewidth.Truncate(b.Title, titleBudget, "...")
			}
			var line1Content string
			if line1Badges != "" {
				line1Content = fmt.Sprintf("%s%s  %s", prefix,
					lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0")).Render(titleTrunc),
					line1Badges)
			} else {
				line1Content = fmt.Sprintf("%s%s", prefix,
					lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0")).Render(titleTrunc))
			}
			line1 := lipgloss.NewStyle().Width(maxWidth).Render(line1Content)

			// Line 2: Rich Meta Info
			metaContent := fmt.Sprintf("%s作者: %s    文库: %s    字数: %s    状态: %s    ID: #%s",
				barInactive, b.Author, pubStr, wordCountStr, statusStr, b.ID)
			line2Trunc := runewidth.Truncate(metaContent, maxWidth, "...")
			line2 := lipgloss.NewStyle().Foreground(theme.TextMuted).Width(maxWidth).Render(line2Trunc)

			// Line 3: Description Preview
			descContent := fmt.Sprintf("%s简介: %s", barInactive, desc)
			descTrunc := runewidth.Truncate(descContent, maxWidth, "...")
			line3 := lipgloss.NewStyle().Foreground(theme.TextDim).Width(maxWidth).Render(descTrunc)

			sb.WriteString(line1 + "\n")
			sb.WriteString(line2 + "\n")
			sb.WriteString(line3 + "\n")
		}
	}

	if end < len(v.books) {
		remaining := len(v.books) - end
		msg := fmt.Sprintf("  ▼ 下方还有 %d 本藏书已折叠 (按 [G] 到底部) ", remaining)
		ruleLen := maxWidth - runewidth.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentAmber).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	} else {
		msg := fmt.Sprintf("  [全部] 已显示全部 %d 本藏书 ", len(v.books))
		ruleLen := maxWidth - runewidth.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentEmerald).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	}

	return sb.String()
}
