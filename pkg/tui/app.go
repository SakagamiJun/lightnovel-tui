package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"lnr-core/pkg/source"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/common"
	"lnr-core/pkg/tui/theme"
	"lnr-core/pkg/tui/views"
)

// AppModel is the root Bubble Tea model managing sub-views, headers, and footer.
type AppModel struct {
	store       *storage.Storage
	src         source.DataSource
	currentView common.ViewID
	width       int
	height      int
	statusText  string

	bookshelfView *views.BookshelfView
	searchView    *views.SearchView
	catalogView   *views.CatalogView
	readerView    *views.ReaderView
}

// NewAppModel initializes root TUI application model.
func NewAppModel(store *storage.Storage, src source.DataSource) *AppModel {
	m := &AppModel{
		store:       store,
		src:         src,
		currentView: common.ViewBookshelf,
		statusText:  "[Tab] 切换书架/在线搜索  │  [↑/↓/滚轮] 选择  │  [Enter] 确认  │  [q] 退出",
	}

	m.bookshelfView = views.NewBookshelfView(store, func(bookID string) tea.Cmd {
		m.currentView = common.ViewCatalog
		return m.catalogView.LoadBook(bookID)
	})

	m.searchView = views.NewSearchView(src, func(bookID string) tea.Cmd {
		m.currentView = common.ViewCatalog
		return m.catalogView.LoadBook(bookID)
	})

	m.catalogView = views.NewCatalogView(store, src, func(bookID, chapterID string) tea.Cmd {
		m.currentView = common.ViewReader
		return m.readerView.OpenChapter(bookID, chapterID)
	})

	m.readerView = views.NewReaderView(store, src)

	return m
}

func (m *AppModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.bookshelfView.Init(),
		m.searchView.Init(),
	)
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentHeight := m.height - 3
		if contentHeight < 1 {
			contentHeight = 1
		}
		m.bookshelfView.SetSize(m.width, contentHeight)
		m.searchView.SetSize(m.width, contentHeight)
		m.catalogView.SetSize(m.width, contentHeight)
		m.readerView.SetSize(m.width, contentHeight)

	case common.StatusMsg:
		m.statusText = string(msg)

	case common.ErrorMsg:
		m.statusText = fmt.Sprintf("❌ 错误: %v", msg)

	case common.SwitchViewMsg:
		m.currentView = msg.Target
		switch msg.Target {
		case common.ViewCatalog:
			cmds = append(cmds, m.catalogView.LoadBook(msg.BookID))
		case common.ViewReader:
			cmds = append(cmds, m.readerView.OpenChapter(msg.BookID, msg.ChapID))
		case common.ViewBookshelf:
			m.bookshelfView.Reload()
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.currentView == common.ViewBookshelf && msg.String() == "q" {
			return m, tea.Quit
		}
		if msg.String() == "tab" && (m.currentView == common.ViewBookshelf || m.currentView == common.ViewSearch) {
			if m.currentView == common.ViewBookshelf {
				m.currentView = common.ViewSearch
			} else {
				m.currentView = common.ViewBookshelf
				m.bookshelfView.Reload()
			}
			return m, nil
		}
	}

	// Route to active sub-view
	var cmd tea.Cmd
	switch m.currentView {
	case common.ViewBookshelf:
		m.bookshelfView, cmd = m.bookshelfView.Update(msg)
		cmds = append(cmds, cmd)
	case common.ViewSearch:
		m.searchView, cmd = m.searchView.Update(msg)
		cmds = append(cmds, cmd)
	case common.ViewCatalog:
		m.catalogView, cmd = m.catalogView.Update(msg)
		cmds = append(cmds, cmd)
	case common.ViewReader:
		m.readerView, cmd = m.readerView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *AppModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "正在加载终端界面..."
	}

	// 1. Top Header Bar with Tabs (strictly 2 lines: tabs + bottom border line)
	title := theme.AppTitleStyle.Render("📖 LNR 轻小说")
	var tabBookshelf, tabSearch string
	if m.currentView == common.ViewBookshelf {
		tabBookshelf = theme.TabActiveStyle.Render("📚 本地书架 (Tab)")
		tabSearch = theme.TabInactiveStyle.Render("🔍 在线搜索 (Tab)")
	} else if m.currentView == common.ViewSearch {
		tabBookshelf = theme.TabInactiveStyle.Render("📚 本地书架 (Tab)")
		tabSearch = theme.TabActiveStyle.Render("🔍 在线搜索 (Tab)")
	} else if m.currentView == common.ViewCatalog {
		tabBookshelf = theme.TabInactiveStyle.Render("📚 本地书架")
		tabSearch = theme.TabActiveStyle.Render("📖 目录分卷")
	} else {
		tabBookshelf = theme.TabInactiveStyle.Render("📚 本地书架")
		tabSearch = theme.TabActiveStyle.Render("👓 沉浸阅读")
	}

	header := lipgloss.JoinHorizontal(lipgloss.Top, title, tabBookshelf, tabSearch)
	headerRendered := theme.HeaderStyle.Width(m.width).Render(header)

	// 2. Bottom Status Bar (strictly 1 line)
	statusBarRendered := theme.StatusBarStyle.Width(m.width).Render(m.statusText)

	// Content budget: total height minus header (2) minus status (1)
	contentHeight := m.height - 3
	if contentHeight < 1 {
		contentHeight = 1
	}

	// 3. Active View Body
	var body string
	switch m.currentView {
	case common.ViewBookshelf:
		body = m.bookshelfView.View()
	case common.ViewSearch:
		body = m.searchView.View()
	case common.ViewCatalog:
		body = m.catalogView.View()
	case common.ViewReader:
		body = m.readerView.View()
	}

	// Split body into lines and clamp/pad to exactly contentHeight lines
	body = strings.TrimSuffix(body, "\n")
	bodyLines := strings.Split(body, "\n")
	if len(bodyLines) > contentHeight {
		bodyLines = bodyLines[:contentHeight]
	}
	for len(bodyLines) < contentHeight {
		bodyLines = append(bodyLines, "")
	}
	finalBody := strings.Join(bodyLines, "\n")

	return fmt.Sprintf("%s\n%s\n%s", headerRendered, finalBody, statusBarRendered)
}
