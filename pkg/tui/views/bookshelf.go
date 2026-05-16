package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"lnr-core/pkg/model"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/common"
	"lnr-core/pkg/tui/theme"
)

// BookshelfView displays locally cached novels with cursor navigation.
type BookshelfView struct {
	store    *storage.Storage
	books    []model.BookDetail
	cursor   int
	offset   int
	width    int
	height   int
	loaded   bool
	onSelect func(bookID string) tea.Cmd
}

// NewBookshelfView constructs bookshelf model.
func NewBookshelfView(store *storage.Storage, onSelect func(bookID string) tea.Cmd) *BookshelfView {
	return &BookshelfView{
		store:    store,
		books:    make([]model.BookDetail, 0),
		onSelect: onSelect,
	}
}

// ReloadLoads cached books from storage.
func (v *BookshelfView) Reload() {
	if v.store != nil {
		books, err := v.store.ListCachedBooks()
		if err == nil {
			v.books = books
		}
	}
	v.loaded = true
}

// SetBooks sets cached books directly (for testing and external feeds).
func (v *BookshelfView) SetBooks(books []model.BookDetail) {
	v.books = books
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

func (v *BookshelfView) Update(msg tea.Msg) (*BookshelfView, tea.Cmd) {
	switch msg := msg.(type) {
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
		case "enter":
			if len(v.books) > 0 && v.cursor < len(v.books) {
				selectedID := v.books[v.cursor].ID
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
		case "r":
			v.Reload()
		}
	}
	return v, nil
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
			Render("📚 本地书架空空如也~\n\n尚未缓存任何轻小说\n按 [Tab] 切换到在线检索，输入书名检索并下载阅读！")
		return emptyBox
	}

	var sb strings.Builder
	curPos := v.cursor + 1
	titleText := fmt.Sprintf(" 📖 本地藏书库 (%d/%d 本)  •  [↑/↓/滚轮] 选择  •  [Enter] 查看目录  •  [r] 刷新", curPos, len(v.books))
	titleBar := lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
		Render(runewidth.Truncate(titleText, maxWidth, "..."))
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

		// Badges
		var statusBadge string
		if b.IsComplete {
			statusBadge = theme.BadgeSuccess.Render("完结")
		} else {
			statusBadge = theme.BadgeWarning.Render("连载")
		}
		pubBadge := theme.BadgeInfo.Render(b.Publisher)
		idBadge := theme.BadgeMuted.Render("#" + b.ID)
		badgeStr := fmt.Sprintf("%s %s %s", statusBadge, pubBadge, idBadge)

		desc := strings.ReplaceAll(b.Description, "\r", " ")
		desc = strings.ReplaceAll(desc, "\n", " ")
		desc = strings.TrimSpace(desc)
		if desc == "" {
			desc = "已缓存到本地，按 [Enter] 查看分卷目录"
		}

		wordCountStr := theme.FormatWordCount(b.WordCount)

		if isSelected {
			// Line 1: Title + Badges
			prefix := barActive
			badgesWidth := runewidth.StringWidth(badgeStr)
			prefixWidth := runewidth.StringWidth(prefix)
			titleBudget := maxWidth - prefixWidth - badgesWidth - 2
			if titleBudget < 8 {
				titleBudget = 8
			}
			titleTrunc := runewidth.Truncate(b.Title, titleBudget, "...")
			line1Content := fmt.Sprintf("%s%s  %s", prefix,
				lipgloss.NewStyle().Bold(true).Foreground(theme.TextWhite).Render(titleTrunc),
				badgeStr)
			line1 := lipgloss.NewStyle().Background(theme.HighlightBg).Width(maxWidth).Render(line1Content)

			// Line 2: Meta Info
			metaContent := fmt.Sprintf("%s👤 作者: %s    📊 字数: %s    💾 状态: 本地已缓存",
				barActiveIndent, b.Author, wordCountStr)
			line2Trunc := runewidth.Truncate(metaContent, maxWidth, "...")
			line2 := lipgloss.NewStyle().Foreground(theme.PrimaryLight).Background(theme.HighlightBg).Width(maxWidth).Render(line2Trunc)

			// Line 3: Description Preview
			descContent := fmt.Sprintf("%s💬 简介: %s", barActiveIndent, desc)
			descTrunc := runewidth.Truncate(descContent, maxWidth, "...")
			line3 := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")).Background(theme.HighlightBg).Width(maxWidth).Render(descTrunc)

			sb.WriteString(line1 + "\n")
			sb.WriteString(line2 + "\n")
			sb.WriteString(line3 + "\n")
		} else {
			// Line 1: Title + Badges
			prefix := barInactive
			badgesWidth := runewidth.StringWidth(badgeStr)
			prefixWidth := runewidth.StringWidth(prefix)
			titleBudget := maxWidth - prefixWidth - badgesWidth - 2
			if titleBudget < 8 {
				titleBudget = 8
			}
			titleTrunc := runewidth.Truncate(b.Title, titleBudget, "...")
			line1Content := fmt.Sprintf("%s%s  %s", prefix,
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0")).Render(titleTrunc),
				badgeStr)
			line1 := lipgloss.NewStyle().Width(maxWidth).Render(line1Content)

			// Line 2: Meta Info
			metaContent := fmt.Sprintf("%s👤 作者: %s    📊 字数: %s    💾 状态: 本地已缓存",
				barInactive, b.Author, wordCountStr)
			line2Trunc := runewidth.Truncate(metaContent, maxWidth, "...")
			line2 := lipgloss.NewStyle().Foreground(theme.TextMuted).Width(maxWidth).Render(line2Trunc)

			// Line 3: Description Preview
			descContent := fmt.Sprintf("%s💬 简介: %s", barInactive, desc)
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
		msg := fmt.Sprintf("  ✓ 已显示全部 %d 本藏书 ", len(v.books))
		ruleLen := maxWidth - runewidth.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentEmerald).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	}

	return sb.String()
}
