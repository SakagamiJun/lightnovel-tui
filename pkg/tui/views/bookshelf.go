package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	books, err := v.store.ListCachedBooks()
	if err == nil {
		v.books = books
	}
	v.loaded = true
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
	// Each book card is ~3 lines high (card + margin)
	cards := (v.height - 4) / 3
	if cards < 2 {
		cards = 2
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

	if len(v.books) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(theme.MutedColor).
			Padding(4, 2).
			Align(lipgloss.Center)
		return emptyStyle.Render("📚 本地书架空空如也~\n\n按 [Tab] 切换到在线搜索，或者输入关键词开始检索小说下载阅读！")
	}

	var sb strings.Builder
	curPos := v.cursor + 1
	titleBar := lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
		Render(fmt.Sprintf("📖 本地藏书 (%d/%d 本) - [↑/↓] 选择, [PgUp/PgDn] 翻页, [Enter] 打开目录, [r] 刷新", curPos, len(v.books)))
	sb.WriteString(titleBar + "\n")

	v.adjustOffset()
	visible := v.visibleCards()
	start := v.offset
	end := start + visible
	if end > len(v.books) {
		end = len(v.books)
	}

	if start > 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render(fmt.Sprintf("  ▲ 上方还有 %d 本藏书被折叠 (按 [g] 到顶部)", start)) + "\n")
	} else {
		sb.WriteString("\n")
	}

	// Render windowed slice of books
	for i := start; i < end; i++ {
		b := v.books[i]
		isSelected := i == v.cursor
		style := theme.CardStyle
		indicator := "  "
		if isSelected {
			style = theme.CardActiveStyle
			indicator = "▶ "
		}

		cardWidth := v.width - 6
		if cardWidth < 30 {
			cardWidth = 30
		}

		status := "连载中"
		if b.IsComplete {
			status = "已完结"
		}

		content := fmt.Sprintf("%s%s  (ID: %s)\n作者: %s | 文库: %s | 状态: %s | %d 字",
			indicator, b.Title, b.ID, b.Author, b.Publisher, status, b.WordCount)

		sb.WriteString(style.Width(cardWidth).Render(content))
		sb.WriteString("\n")
	}

	if end < len(v.books) {
		remaining := len(v.books) - end
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render(fmt.Sprintf("  ▼ 下方还有 %d 本藏书被折叠 (按 [G] 到底部)", remaining)) + "\n")
	}

	return sb.String()
}
