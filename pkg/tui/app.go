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
)

// View represents an interactive sub-view in the TUI application.
type View interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (View, tea.Cmd)
	View() string
	SetSize(width, height int)
}

// AppModel is the root Bubble Tea model managing sub-views, headers, and footer.
type AppModel struct {
	store       *storage.Storage
	src         source.DataSource
	currentView common.ViewID
	width       int
	height      int
	statusText  string

	// Sub-views map or interfaces
	bookshelfView View
	searchView    View
	catalogView   View
	readerView    View
}

// NewAppModel initializes root TUI application model.
func NewAppModel(store *storage.Storage, src source.DataSource) *AppModel {
	return &AppModel{
		store:       store,
		src:         src,
		currentView: common.ViewBookshelf,
		statusText:  "欢迎使用 LightNovelReader TUI | 按 [Tab] 切换书架/搜索，按 [q] 退出",
	}
}

func (m *AppModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
	)
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentHeight := m.height - 4 // minus header and status bar
		if m.bookshelfView != nil {
			m.bookshelfView.SetSize(m.width, contentHeight)
		}
		if m.searchView != nil {
			m.searchView.SetSize(m.width, contentHeight)
		}
		if m.catalogView != nil {
			m.catalogView.SetSize(m.width, contentHeight)
		}
		if m.readerView != nil {
			m.readerView.SetSize(m.width, contentHeight)
		}

	case common.StatusMsg:
		m.statusText = string(msg)

	case common.ErrorMsg:
		m.statusText = fmt.Sprintf("❌ 错误: %v", msg)

	case common.SwitchViewMsg:
		m.currentView = msg.Target
		// Handle specific view transitions if needed

	case tea.KeyMsg:
		// Global hotkeys
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.currentView == common.ViewBookshelf {
				m.currentView = common.ViewSearch
			} else if m.currentView == common.ViewSearch {
				m.currentView = common.ViewBookshelf
			}
			return m, nil
		}
	}

	// Route to active sub-view
	var cmd tea.Cmd
	switch m.currentView {
	case common.ViewBookshelf:
		if m.bookshelfView != nil {
			m.bookshelfView, cmd = m.bookshelfView.Update(msg)
			cmds = append(cmds, cmd)
		}
	case common.ViewSearch:
		if m.searchView != nil {
			m.searchView, cmd = m.searchView.Update(msg)
			cmds = append(cmds, cmd)
		}
	case common.ViewCatalog:
		if m.catalogView != nil {
			m.catalogView, cmd = m.catalogView.Update(msg)
			cmds = append(cmds, cmd)
		}
	case common.ViewReader:
		if m.readerView != nil {
			m.readerView, cmd = m.readerView.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *AppModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "正在加载终端界面..."
	}

	var sb strings.Builder

	// 1. Top Header Bar with Tabs
	title := theme.AppTitleStyle.Render("📖 LNR 轻小说")
	var tabBookshelf, tabSearch string
	if m.currentView == common.ViewBookshelf {
		tabBookshelf = theme.TabActiveStyle.Render("📚 本地书架 (1)")
		tabSearch = theme.TabInactiveStyle.Render("🔍 在线搜索 (2)")
	} else if m.currentView == common.ViewSearch {
		tabBookshelf = theme.TabInactiveStyle.Render("📚 本地书架 (1)")
		tabSearch = theme.TabActiveStyle.Render("🔍 在线搜索 (2)")
	} else if m.currentView == common.ViewCatalog {
		tabBookshelf = theme.TabInactiveStyle.Render("📚 本地书架")
		tabSearch = theme.TabActiveStyle.Render("📖 小说目录")
	} else {
		tabBookshelf = theme.TabInactiveStyle.Render("📚 本地书架")
		tabSearch = theme.TabActiveStyle.Render("👓 沉浸阅读")
	}

	header := lipgloss.JoinHorizontal(lipgloss.Top, title, tabBookshelf, tabSearch)
	sb.WriteString(theme.HeaderStyle.Width(m.width).Render(header))
	sb.WriteString("\n")

	// 2. Active View Body
	var body string
	switch m.currentView {
	case common.ViewBookshelf:
		if m.bookshelfView != nil {
			body = m.bookshelfView.View()
		} else {
			body = "正在加载书架..."
		}
	case common.ViewSearch:
		if m.searchView != nil {
			body = m.searchView.View()
		} else {
			body = "正在加载搜索..."
		}
	case common.ViewCatalog:
		if m.catalogView != nil {
			body = m.catalogView.View()
		} else {
			body = "正在加载目录..."
		}
	case common.ViewReader:
		if m.readerView != nil {
			body = m.readerView.View()
		} else {
			body = "正在加载正文..."
		}
	}
	sb.WriteString(body)
	sb.WriteString("\n")

	// 3. Bottom Status Bar
	statusBar := theme.StatusBarStyle.Width(m.width).Render(m.statusText)
	sb.WriteString(statusBar)

	return sb.String()
}
