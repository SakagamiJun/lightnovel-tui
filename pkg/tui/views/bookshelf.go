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
	// Each book item takes exactly 2 lines.
	avail := v.height - 3
	if avail < 2 {
		return 1
	}
	cards := avail / 2
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
		Render(fmt.Sprintf("📖 本地藏书 (%d/%d 本) - [↑/↓/滚轮] 选择, [Enter] 打开目录, [r] 刷新", curPos, len(v.books)))
	sb.WriteString(titleBar + "\n")

	v.adjustOffset()
	visible := v.visibleCards()
	start := v.offset
	end := start + visible
	if end > len(v.books) {
		end = len(v.books)
	}

	maxWidth := v.width - 2
	if maxWidth < 30 {
		maxWidth = 30
	}

	if start > 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render(fmt.Sprintf("  ▲ 上方还有 %d 本藏书被折叠 (按 [g] 到顶部)", start)) + "\n")
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.BorderColor).
			Render(strings.Repeat("─", maxWidth)) + "\n")
	}

	// Render windowed slice of books
	for i := start; i < end; i++ {
		b := v.books[i]
		isSelected := i == v.cursor
		indicator := "  "
		if isSelected {
			indicator = "▶ "
		}

		status := "连载中"
		if b.IsComplete {
			status = "已完结"
		}

		desc := strings.ReplaceAll(b.Description, "\r", " ")
		desc = strings.ReplaceAll(desc, "\n", " ")
		desc = strings.TrimSpace(desc)
		if desc == "" {
			desc = "已缓存到本地，按 [Enter] 查看分卷目录"
		} else {
			desc = "简介: " + desc
		}

		line1Raw := fmt.Sprintf("%s%s (ID: %s)  %s · %s · %s · %d字",
			indicator, b.Title, b.ID, b.Author, b.Publisher, status, b.WordCount)
		line2Raw := fmt.Sprintf("    %s", desc)

		line1Trunc := runewidth.Truncate(line1Raw, maxWidth, "...")
		line2Trunc := runewidth.Truncate(line2Raw, maxWidth, "...")

		if isSelected {
			sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).
				Background(theme.HighlightBg).Width(maxWidth).Render(line1Trunc))
			sb.WriteString("\n")
			sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentColor).
				Background(theme.HighlightBg).Width(maxWidth).Render(line2Trunc))
			sb.WriteString("\n")
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC")).
				Width(maxWidth).Render(line1Trunc))
			sb.WriteString("\n")
			sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
				Width(maxWidth).Render(line2Trunc))
			sb.WriteString("\n")
		}
	}

	if end < len(v.books) {
		remaining := len(v.books) - end
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render(fmt.Sprintf("  ▼ 下方还有 %d 本藏书被折叠 (按 [G] 到底部)", remaining)) + "\n")
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render("  ✓ 已显示到底部") + "\n")
	}

	return sb.String()
}
