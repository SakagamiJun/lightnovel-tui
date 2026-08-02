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
	store             *storage.Storage
	src               source.DataSource
	allBooks          []model.BookDetail
	books             []model.BookDetail
	shelves           []storage.BookshelfGroup
	activeShelfIdx    int
	showMoveModal     bool
	moveCursor        int
	pinnedSet         map[string]bool
	cursor            int
	offset            int
	width             int
	height            int
	loaded            bool
	confirmDelete     bool
	sortCriteria      storage.SortCriteria
	checkingUpdates   bool
	updatesMap        map[string]int
	showStatsModal    bool
	onSelect          func(bookID string) tea.Cmd
	onContinueReading func(bookID, chapterID string) tea.Cmd
	onExport          func(bookID string, volumeIndex int) tea.Cmd
}

// NewBookshelfView constructs bookshelf model.
func NewBookshelfView(store *storage.Storage, onSelect func(bookID string) tea.Cmd) *BookshelfView {
	return &BookshelfView{
		store:        store,
		allBooks:     make([]model.BookDetail, 0),
		books:        make([]model.BookDetail, 0),
		shelves:      storage.DefaultBookshelves,
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

// SetOnContinueReading registers callback to jump straight into reading.
func (v *BookshelfView) SetOnContinueReading(fn func(bookID, chapterID string) tea.Cmd) {
	v.onContinueReading = fn
}

// SetOnExport registers export callback.
func (v *BookshelfView) SetOnExport(fn func(bookID string, volumeIndex int) tea.Cmd) {
	v.onExport = fn
}

// Reload loads cached books from storage and filters by the active bookshelf category.
func (v *BookshelfView) Reload() {
	if v.store != nil {
		all, err := v.store.ListCachedBooks()
		if err == nil {
			v.allBooks = all
			pinnedIDs, _ := v.store.GetPinnedBookIDs()
			v.pinnedSet = make(map[string]bool, len(pinnedIDs))
			for _, id := range pinnedIDs {
				v.pinnedSet[id] = true
			}

			// Load shelf groups
			shelves, err := v.store.LoadShelves()
			if err == nil && len(shelves) > 0 {
				v.shelves = shelves
			} else {
				v.shelves = storage.DefaultBookshelves
			}

			if v.activeShelfIdx >= len(v.shelves) {
				v.activeShelfIdx = 0
			}

			// Filter by active shelf
			filtered := make([]model.BookDetail, 0)
			if v.activeShelfIdx == 0 || v.shelves[v.activeShelfIdx].ID == "all" {
				filtered = make([]model.BookDetail, len(v.allBooks))
				copy(filtered, v.allBooks)
			} else {
				shelfBookIDs := make(map[string]bool)
				for _, id := range v.shelves[v.activeShelfIdx].BookIDs {
					shelfBookIDs[id] = true
				}
				for _, b := range v.allBooks {
					if shelfBookIDs[b.ID] {
						filtered = append(filtered, b)
					}
				}
			}

			// Sort books using active sort criteria and pinned status
			v.store.SortBooks(filtered, v.sortCriteria)
			v.books = filtered
		}
	}
	if v.pinnedSet == nil {
		v.pinnedSet = make(map[string]bool)
	}
	if v.cursor >= len(v.books) && len(v.books) > 0 {
		v.cursor = len(v.books) - 1
	} else if len(v.books) == 0 {
		v.cursor = 0
		v.offset = 0
	}
	v.loaded = true
}

// SetBooks sets cached books directly (for testing and external feeds).
func (v *BookshelfView) SetBooks(books []model.BookDetail) {
	v.allBooks = books
	v.books = books
	if len(v.shelves) == 0 {
		v.shelves = storage.DefaultBookshelves
	}
	if v.pinnedSet == nil {
		v.pinnedSet = make(map[string]bool)
	}
	v.loaded = true
	v.cursor = 0
	v.offset = 0
}

// IsConfirmingDelete returns whether the view is awaiting delete confirmation.
func (v *BookshelfView) IsConfirmingDelete() bool {
	return v.confirmDelete
}

// IsShowingStats returns whether the reading statistics modal is currently active.
func (v *BookshelfView) IsShowingStats() bool {
	return v.showStatsModal
}

// CloseStats dismisses the reading statistics modal.
func (v *BookshelfView) CloseStats() {
	v.showStatsModal = false
}

// IsMovingBook returns whether the move bookshelf modal is currently open.
func (v *BookshelfView) IsMovingBook() bool {
	return v.showMoveModal
}

// CloseMoveModal dismisses the move bookshelf modal.
func (v *BookshelfView) CloseMoveModal() {
	v.showMoveModal = false
}

func (v *BookshelfView) targetShelves() []storage.BookshelfGroup {
	targets := make([]storage.BookshelfGroup, 0, len(v.shelves))
	for _, sh := range v.shelves {
		if sh.ID != "all" {
			targets = append(targets, sh)
		}
	}
	return targets
}

func (v *BookshelfView) executeMoveBook(target storage.BookshelfGroup) tea.Cmd {
	if v.cursor >= len(v.books) {
		v.showMoveModal = false
		return nil
	}
	b := v.books[v.cursor]
	v.showMoveModal = false
	if v.store != nil {
		_ = v.store.MoveBookToShelf(b.ID, target.ID)
	}
	v.Reload()
	return func() tea.Msg {
		return common.StatusMsg(fmt.Sprintf("已将《%s》移至书架「%s」", b.Title, target.Name))
	}
}

func (v *BookshelfView) renderMoveModal() string {
	maxWidth := v.width - 6
	if maxWidth > 56 {
		maxWidth = 56
	}
	if maxWidth < 32 {
		maxWidth = 32
	}

	selectedTitle := ""
	if v.cursor < len(v.books) {
		selectedTitle = v.books[v.cursor].Title
	}

	var sb strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
		Render(fmt.Sprintf("[移动藏书分组] 《%s》", runewidth.Truncate(selectedTitle, 24, "...")))
	sb.WriteString(title + "\n\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.TextMuted).Render("选择目标书架分类:") + "\n\n")

	targetShelves := v.targetShelves()
	for i, sh := range targetShelves {
		isSelected := i == v.moveCursor
		keyNum := fmt.Sprintf("[%d]", i+1)
		count := len(sh.BookIDs)
		if isSelected {
			line := fmt.Sprintf("  ▎ ▶ %s %s  %s",
				lipgloss.NewStyle().Bold(true).Foreground(theme.AccentSky).Render(keyNum),
				lipgloss.NewStyle().Bold(true).Foreground(theme.TextWhite).Render(sh.Name),
				lipgloss.NewStyle().Foreground(theme.AccentAmber).Render(fmt.Sprintf("(当前 %d 本)", count)))
			sb.WriteString(line + "\n")
		} else {
			line := fmt.Sprintf("    │ %s %s  %s",
				lipgloss.NewStyle().Foreground(theme.TextMuted).Render(keyNum),
				lipgloss.NewStyle().Foreground(theme.TextWhite).Render(sh.Name),
				lipgloss.NewStyle().Foreground(theme.TextDim).Render(fmt.Sprintf("(当前 %d 本)", count)))
			sb.WriteString(line + "\n")
		}
	}

	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(theme.BorderColor).Render(strings.Repeat("─", maxWidth-4)) + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.TextMuted).Render("[1-9/↑/↓] 选择  •  [Enter] 确认移组  •  [Esc] 取消"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.PrimaryColor).
		Padding(1, 2).
		Width(maxWidth).
		Render(sb.String())
}

func (v *BookshelfView) resumeRecentBook() tea.Cmd {
	if v.store == nil {
		return func() tea.Msg { return common.StatusMsg("存储未就绪") }
	}
	recent := v.store.GetRecentBook()
	if recent == nil || recent.BookID == "" {
		return func() tea.Msg { return common.StatusMsg("暂无最近阅读记录，请先选择一本书籍阅读") }
	}
	v.showStatsModal = false
	if v.onContinueReading != nil {
		return v.onContinueReading(recent.BookID, recent.LastReadChapID)
	}
	if v.onSelect != nil {
		return v.onSelect(recent.BookID)
	}
	return func() tea.Msg {
		return common.SwitchViewMsg{
			Target: common.ViewCatalog,
			BookID: recent.BookID,
		}
	}
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
	// Fixed lines: TitleBar (1) + GroupsBar (1) + TopInd (1) + BotInd (1) = 4 lines.
	// Each modern card takes exactly 3 lines.
	avail := v.height - 4
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
		if v.showMoveModal {
			targets := v.targetShelves()
			switch msg.String() {
			case "esc":
				v.showMoveModal = false
				return v, nil
			case "up", "k":
				if v.moveCursor > 0 {
					v.moveCursor--
				}
				return v, nil
			case "down", "j":
				if v.moveCursor < len(targets)-1 {
					v.moveCursor++
				}
				return v, nil
			case "1", "2", "3", "4", "5", "6", "7", "8", "9":
				idx := int(msg.String()[0] - '1')
				if idx >= 0 && idx < len(targets) {
					v.moveCursor = idx
					return v, v.executeMoveBook(targets[idx])
				}
				return v, nil
			case "enter":
				if len(targets) > 0 && v.moveCursor < len(targets) {
					return v, v.executeMoveBook(targets[v.moveCursor])
				}
				return v, nil
			default:
				return v, nil
			}
		}

		if v.showStatsModal {
			switch msg.String() {
			case "esc", "s", "S":
				v.showStatsModal = false
				return v, nil
			case "enter", "c", "C":
				return v, v.resumeRecentBook()
			default:
				return v, nil
			}
		}

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
		case "g":
			if len(v.shelves) > 0 {
				v.activeShelfIdx = (v.activeShelfIdx + 1) % len(v.shelves)
				v.Reload()
				v.cursor = 0
				v.offset = 0
				return v, func() tea.Msg {
					return common.StatusMsg(fmt.Sprintf("已切换至书架分组「%s」", v.shelves[v.activeShelfIdx].Name))
				}
			}
		case "G":
			if len(v.shelves) > 0 {
				v.activeShelfIdx = (v.activeShelfIdx - 1 + len(v.shelves)) % len(v.shelves)
				v.Reload()
				v.cursor = 0
				v.offset = 0
				return v, func() tea.Msg {
					return common.StatusMsg(fmt.Sprintf("已切换至书架分组「%s」", v.shelves[v.activeShelfIdx].Name))
				}
			}
		case "home":
			v.cursor = 0
			v.offset = 0
		case "end":
			if len(v.books) > 0 {
				v.cursor = len(v.books) - 1
				v.adjustOffset()
			}
		case "m", "M":
			if len(v.books) > 0 && v.cursor < len(v.books) {
				v.showMoveModal = true
				v.moveCursor = 0
				return v, nil
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
		case "E":
			if len(v.books) > 0 && v.cursor < len(v.books) && v.onExport != nil {
				return v, v.onExport(v.books[v.cursor].ID, common.ExportModeAllVolumesSeparate)
			}
		case "s", "S":
			v.showStatsModal = true
			return v, nil
		case "c", "C":
			return v, v.resumeRecentBook()
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

	if v.showStatsModal {
		return RenderStatsModal(v.store, v.width, v.height)
	}

	if v.showMoveModal {
		return v.renderMoveModal()
	}

	maxWidth := v.width - 2
	if maxWidth < 30 {
		maxWidth = 30
	}

	var sb strings.Builder
	curPos := v.cursor + 1
	var titleBar string
	if v.confirmDelete && v.cursor < len(v.books) {
		delPrompt := fmt.Sprintf(" [确认删除] 确认删除《%s》本地缓存？按 [y] 确认 / 按 [n/Esc] 取消", v.books[v.cursor].Title)
		titleBar = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentRose).
			Render(theme.TruncateANSI(delPrompt, maxWidth, "..."))
	} else {
		sortName := storage.SortCriteriaNames[v.sortCriteria]
		var updateStatus string
		if v.checkingUpdates {
			updateStatus = " • [检查中...]"
		}
		var titleText string
		if maxWidth >= 105 {
			titleText = fmt.Sprintf(" 本地藏书库 (%d/%d 本) • [g/G]分组 • [m]移组 • [s]统计 • [c]续读 • [排序: %s(o)] • [u]更新 • [p]置顶 • [x]删除%s",
				curPos, len(v.books), sortName, updateStatus)
		} else if maxWidth >= 80 {
			titleText = fmt.Sprintf(" 本地藏书库 (%d/%d) • [g/G]分组 • [m]移组 • [c]续读 • [排序: %s(o)] • [u]更新 • [p]置顶%s",
				curPos, len(v.books), sortName, updateStatus)
		} else {
			titleText = fmt.Sprintf(" 本地藏书库 (%d/%d) • [%s] • [g/G]分组 • [m]移组%s",
				curPos, len(v.books), sortName, updateStatus)
		}
		titleBar = lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
			Render(theme.TruncateANSI(titleText, maxWidth, "..."))
	}
	sb.WriteString(titleBar + "\n")

	// Bookshelf Groups Bar
	if len(v.shelves) > 0 {
		var shelfTabs []string
		for i, sh := range v.shelves {
			count := 0
			if sh.ID == "all" {
				count = len(v.allBooks)
			} else {
				idSet := make(map[string]bool)
				for _, id := range sh.BookIDs {
					idSet[id] = true
				}
				for _, b := range v.allBooks {
					if idSet[b.ID] {
						count++
					}
				}
			}
			label := fmt.Sprintf("[%s (%d)]", sh.Name, count)
			if i == v.activeShelfIdx {
				shelfTabs = append(shelfTabs, lipgloss.NewStyle().
					Bold(true).
					Foreground(theme.TextWhite).
					Background(theme.SecondaryColor).
					Padding(0, 1).
					Render(label))
			} else {
				shelfTabs = append(shelfTabs, lipgloss.NewStyle().
					Foreground(theme.TextMuted).
					Background(theme.BarBg).
					Padding(0, 1).
					Render(label))
			}
		}
		var groupsBar string
		if maxWidth >= 85 {
			groupsBar = " 分组: " + strings.Join(shelfTabs, " ") + "  " +
				lipgloss.NewStyle().Foreground(theme.TextDim).Render("(按 [g/G] 快速轮换)")
		} else {
			groupsBar = " 分组: " + strings.Join(shelfTabs, " ")
		}
		sb.WriteString(theme.TruncateANSI(groupsBar, maxWidth, "...") + "\n")
	}

	if len(v.books) == 0 {
		shelfName := "全部"
		if v.activeShelfIdx < len(v.shelves) {
			shelfName = v.shelves[v.activeShelfIdx].Name
		}
		var emptyPrompt string
		if v.activeShelfIdx == 0 {
			emptyPrompt = "本地书架暂无藏书\n\n尚未缓存任何轻小说\n按 [s] 查看阅读统计与打卡热力图\n按 [Tab] 切换到在线检索，输入书名检索并下载阅读！"
		} else {
			emptyPrompt = fmt.Sprintf("当前书架「%s」暂无藏书\n\n按 [g/G] 切换其他书架分组\n选中小说按 [m] 可自由移入此分类", shelfName)
		}
		sb.WriteString(lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.BorderColor).
			Padding(2, 4).
			Align(lipgloss.Center).
			Render(emptyPrompt))
		return sb.String()
	}

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

		desc := theme.CleanDescription(b.Description)
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
				titleTrunc = theme.TruncateANSI(b.Title, titleBudget, "...")
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

			// Line 2: Rich Meta Info (Adaptive Spacing)
			var metaContent string
			if maxWidth >= 100 {
				metaContent = fmt.Sprintf("%s作者: %s    文库: %s    字数: %s    状态: %s    ID: #%s",
					barActiveIndent, b.Author, pubStr, wordCountStr, statusStr, b.ID)
			} else if maxWidth >= 80 {
				metaContent = fmt.Sprintf("%s作者: %s  文库: %s  字数: %s  状态: %s  #%s",
					barActiveIndent, b.Author, pubStr, wordCountStr, statusStr, b.ID)
			} else {
				metaContent = fmt.Sprintf("%s%s • %s • %s • %s",
					barActiveIndent, b.Author, pubStr, wordCountStr, statusStr)
			}
			line2Trunc := theme.TruncateANSI(metaContent, maxWidth, "...")
			line2 := lipgloss.NewStyle().Foreground(theme.PrimaryLight).Background(theme.HighlightBg).Width(maxWidth).Render(line2Trunc)

			// Line 3: Description Preview
			descContent := fmt.Sprintf("%s简介: %s", barActiveIndent, desc)
			descTrunc := theme.TruncateANSI(descContent, maxWidth, "...")
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
				titleTrunc = theme.TruncateANSI(b.Title, titleBudget, "...")
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

			// Line 2: Rich Meta Info (Adaptive Spacing)
			var metaContent string
			if maxWidth >= 100 {
				metaContent = fmt.Sprintf("%s作者: %s    文库: %s    字数: %s    状态: %s    ID: #%s",
					barInactive, b.Author, pubStr, wordCountStr, statusStr, b.ID)
			} else if maxWidth >= 80 {
				metaContent = fmt.Sprintf("%s作者: %s  文库: %s  字数: %s  状态: %s  #%s",
					barInactive, b.Author, pubStr, wordCountStr, statusStr, b.ID)
			} else {
				metaContent = fmt.Sprintf("%s%s • %s • %s • %s",
					barInactive, b.Author, pubStr, wordCountStr, statusStr)
			}
			line2Trunc := theme.TruncateANSI(metaContent, maxWidth, "...")
			line2 := lipgloss.NewStyle().Foreground(theme.TextMuted).Width(maxWidth).Render(line2Trunc)

			// Line 3: Description Preview
			descContent := fmt.Sprintf("%s简介: %s", barInactive, desc)
			descTrunc := theme.TruncateANSI(descContent, maxWidth, "...")
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
