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

func (v *CatalogView) Update(msg tea.Msg) (*CatalogView, tea.Cmd) {
	switch msg := msg.(type) {
	case catalogResultMsg:
		v.loading = false
		v.detail = msg.detail
		v.catalog = msg.catalog
		v.err = msg.err
		v.flattenItems()
		return v, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if v.cursor > 0 {
				v.cursor--
			}
		case "down", "j":
			if v.cursor < len(v.flatItems)-1 {
				v.cursor++
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

	var sb strings.Builder
	if v.detail != nil {
		header := lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
			Render(fmt.Sprintf("📖 %s (作者: %s) - [Esc] 返回, [Enter] 开始阅读", v.detail.Title, v.detail.Author))
		sb.WriteString(header + "\n\n")
	}

	for i, item := range v.flatItems {
		isSelected := i == v.cursor
		if item.isVolume {
			volStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.AccentColor).MarginTop(1)
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

	return sb.String()
}
