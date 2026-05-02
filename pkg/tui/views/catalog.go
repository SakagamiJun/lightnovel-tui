package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"lnr-core/pkg/model"
	"lnr-core/pkg/source"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/common"
	"lnr-core/pkg/tui/theme"
)

type catalogResultMsg struct {
	detail  *model.BookDetail
	catalog *model.BookCatalog
	err     error
}

// CatalogView renders the volume and chapter tree of a novel.
type CatalogView struct {
	store     *storage.Storage
	src       source.DataSource
	bookID    string
	detail    *model.BookDetail
	catalog   *model.BookCatalog
	cursor    int
	offset    int
	flatItems []flatChapterItem
	loading   bool
	err       error
	width     int
	height    int
	onSelect  func(bookID, chapterID string) tea.Cmd
}

type flatChapterItem struct {
	isVolume bool
	volTitle string
	chapID   string
	title    string
}

// NewCatalogView constructs a catalog navigation view.
func NewCatalogView(store *storage.Storage, src source.DataSource, onSelect func(bookID, chapterID string) tea.Cmd) *CatalogView {
	return &CatalogView{
		store:     store,
		src:       src,
		flatItems: make([]flatChapterItem, 0),
		onSelect:  onSelect,
	}
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
	// Total available height minus top title (2 lines) and bottom indicators
	lines := v.height - 4
	if lines < 5 {
		lines = 5
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

func (v *CatalogView) Update(msg tea.Msg) (*CatalogView, tea.Cmd) {
	switch msg := msg.(type) {
	case catalogResultMsg:
		v.loading = false
		v.detail = msg.detail
		v.catalog = msg.catalog
		v.err = msg.err
		v.cursor = 0
		v.offset = 0
		v.flattenItems()
		return v, nil

	case tea.MouseMsg:
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
		case "esc":
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
	for _, vol := range v.catalog.Volumes {
		v.flatItems = append(v.flatItems, flatChapterItem{
			isVolume: true,
			volTitle: vol.Title,
			title:    vol.Title,
		})
		for _, ch := range vol.Chapters {
			v.flatItems = append(v.flatItems, flatChapterItem{
				isVolume: false,
				volTitle: vol.Title,
				chapID:   ch.ID,
				title:    ch.Title,
			})
		}
	}
}

func (v *CatalogView) View() string {
	if v.loading {
		return "\n⏳ 正在获取小说分卷目录，请稍候..."
	}
	if v.err != nil {
		return fmt.Sprintf("\n❌ 获取目录失败: %v\n按 [Esc] 返回书架", v.err)
	}

	if len(v.flatItems) == 0 {
		return "\n📖 暂无目录内容。按 [Esc] 返回书架"
	}

	var sb strings.Builder
	if v.detail != nil {
		curPos := v.cursor + 1
		total := len(v.flatItems)
		header := lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
			Render(fmt.Sprintf("📖 %s (%d/%d 项) - [↑/↓]移动, [PgUp/PgDn]翻页, [g/G]顶/底, [Enter]阅读, [Esc]返回",
				v.detail.Title, curPos, total))
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
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render(fmt.Sprintf("  ▲ 上方还有 %d 项被折叠 (按 [g] 回到顶部)", start)) + "\n")
	} else {
		sb.WriteString("\n")
	}

	// Windowed slice rendering
	for i := start; i < end; i++ {
		item := v.flatItems[i]
		isSelected := i == v.cursor
		if item.isVolume {
			volStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.AccentColor)
			sb.WriteString(volStyle.Render("📁 " + item.volTitle))
			sb.WriteString("\n")
		} else {
			chStyle := lipgloss.NewStyle()
			prefix := "    "
			if isSelected {
				chStyle = chStyle.Bold(true).Foreground(theme.PrimaryColor).Background(theme.HighlightBg)
				prefix = "  ▶ "
			}
			sb.WriteString(chStyle.Render(prefix + item.title))
			sb.WriteString("\n")
		}
	}

	// Bottom indicator if truncated
	if end < len(v.flatItems) {
		remaining := len(v.flatItems) - end
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render(fmt.Sprintf("  ▼ 下方还有 %d 项被折叠 (按 [G] 跳到底部)", remaining)) + "\n")
	}

	return sb.String()
}
