package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SakagamiJun/lightnovel-tui/pkg/model"
	"github.com/SakagamiJun/lightnovel-tui/pkg/source"
	"github.com/SakagamiJun/lightnovel-tui/pkg/storage"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/common"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/theme"
)

type searchResultMsg struct {
	query      string
	results    []model.BookSummary
	totalPages int
	page       int
	err        error
}

type searchDescEnrichedMsg struct {
	query string
	page  int
	items map[string]string
}

// SearchView handles interactive search with textinput.
type SearchView struct {
	src        source.DataSource
	store      *storage.Storage
	input      textinput.Model
	results    []model.BookSummary
	cursor     int
	offset     int
	searching  bool
	err        error
	width      int
	height     int
	page       int
	totalPages int
	onSelect   func(bookID string) tea.Cmd
	onDownload func(bookID string) tea.Cmd
	onExport   func(bookID string, volumeIndex int) tea.Cmd
}

// NewSearchView creates an interactive search view.
func NewSearchView(src source.DataSource, store *storage.Storage, onSelect func(bookID string) tea.Cmd) *SearchView {
	ti := textinput.New()
	ti.Placeholder = "输入书名或作者名，按 [Enter] 开始检索..."
	ti.Focus()
	ti.CharLimit = 50
	ti.Width = 46
	ti.Prompt = " 检索轻小说: "
	ti.PromptStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryLight)
	ti.TextStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.TextWhite)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.TextDim)

	return &SearchView{
		src:        src,
		store:      store,
		input:      ti,
		results:    make([]model.BookSummary, 0),
		page:       1,
		totalPages: 1,
		onSelect:   onSelect,
	}
}

// SetOnDownload registers download callback.
func (v *SearchView) SetOnDownload(fn func(bookID string) tea.Cmd) {
	v.onDownload = fn
}

// SetOnExport registers export callback.
func (v *SearchView) SetOnExport(fn func(bookID string, volumeIndex int) tea.Cmd) {
	v.onExport = fn
}

// Init initializes search text input.
func (v *SearchView) Init() tea.Cmd {
	return textinput.Blink
}

// SetSize updates layout dimensions.
func (v *SearchView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// IsInputFocused returns whether the search input currently has focus.
func (v *SearchView) IsInputFocused() bool {
	return v.input.Focused()
}

// SetResults populates search results directly.
func (v *SearchView) SetResults(results []model.BookSummary) {
	v.searching = false
	v.results = results
	v.err = nil
	v.cursor = 0
	v.offset = 0
	v.page = 1
	v.totalPages = 1
	if len(results) > 0 {
		v.input.Blur()
	}
}

func (v *SearchView) visibleCards() int {
	// Fixed lines:
	// Line 1: Header (1)
	// Line 2: Tips (1)
	// Line 3: Input (1)
	// Line 4: Stats (1)
	// Line 5: TopInd (1)
	// Line N: BotInd (1)
	// Total fixed lines = 6 lines.
	// Each modern card takes exactly 3 lines.
	avail := v.height - 6
	if avail < 3 {
		return 1
	}
	cards := avail / 3
	if cards < 1 {
		cards = 1
	}
	return cards
}

func (v *SearchView) adjustOffset() {
	visible := v.visibleCards()
	if v.cursor < v.offset {
		v.offset = v.cursor
	} else if v.cursor >= v.offset+visible {
		v.offset = v.cursor - visible + 1
	}
	if v.offset < 0 {
		v.offset = 0
	}
	maxOffset := len(v.results) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if v.offset > maxOffset {
		v.offset = maxOffset
	}
}

func (v *SearchView) quickFillFromStore() {
	if v.store == nil {
		return
	}
	for i := range v.results {
		b := &v.results[i]
		clean := strings.TrimSpace(b.Description)
		if clean == "" || strings.HasSuffix(clean, "…") || strings.HasSuffix(clean, "...") || strings.HasSuffix(clean, "─…") || len([]rune(clean)) <= 60 {
			if detail, err := v.store.LoadBookDetail(b.ID); err == nil && detail != nil && detail.Description != "" {
				b.Description = detail.Description
			}
		}
	}
}

func (v *SearchView) enrichDescriptionsCmd(query string, page int, books []model.BookSummary) tea.Cmd {
	if v.src == nil || len(books) == 0 {
		return nil
	}

	booksCopy := make([]model.BookSummary, len(books))
	copy(booksCopy, books)

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		enriched := v.src.EnrichDescriptions(ctx, booksCopy, 6)
		items := make(map[string]string, len(enriched))
		for _, b := range enriched {
			if b.Description != "" {
				items[b.ID] = b.Description
				if v.store != nil {
					// Persist if full detail already in store or save
					if detail, err := v.store.LoadBookDetail(b.ID); err == nil && detail != nil {
						if detail.Description == "" {
							detail.Description = b.Description
							_ = v.store.SaveBookDetail(detail)
						}
					}
				}
			}
		}
		return searchDescEnrichedMsg{
			query: query,
			page:  page,
			items: items,
		}
	}
}

// Update handles input events in SearchView.
func (v *SearchView) Update(msg tea.Msg) (*SearchView, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case searchResultMsg:
		v.searching = false
		v.results = msg.results
		v.totalPages = msg.totalPages
		v.page = msg.page
		v.err = msg.err
		v.cursor = 0
		v.offset = 0
		if len(msg.results) > 0 {
			v.input.Blur()
			v.quickFillFromStore()
			return v, v.enrichDescriptionsCmd(msg.query, msg.page, v.results)
		}
		return v, nil

	case searchDescEnrichedMsg:
		currQuery := strings.TrimSpace(v.input.Value())
		if msg.query == currQuery && msg.page == v.page {
			for i := range v.results {
				if fullDesc, ok := msg.items[v.results[i].ID]; ok && fullDesc != "" {
					v.results[i].Description = fullDesc
				}
			}
		}
		return v, nil

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if len(v.results) > 0 {
				if v.input.Focused() {
					v.input.Blur()
				}
				if v.cursor > 0 {
					v.cursor--
					v.adjustOffset()
				}
			}
		case tea.MouseButtonWheelDown:
			if len(v.results) > 0 {
				if v.input.Focused() {
					v.input.Blur()
				}
				if v.cursor < len(v.results)-1 {
					v.cursor++
					v.adjustOffset()
				}
			}
		}

	case tea.KeyMsg:
		visible := v.visibleCards()
		switch msg.String() {
		case "enter":
			if v.input.Focused() && strings.TrimSpace(v.input.Value()) != "" {
				v.searching = true
				v.err = nil
				v.page = 1
				query := strings.TrimSpace(v.input.Value())
				return v, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					if v.src == nil {
						return searchResultMsg{query: query, results: nil, totalPages: 1, page: 1, err: nil}
					}
					res, total, err := v.src.Search(ctx, source.SearchTypeTitle, query, 1)
					return searchResultMsg{query: query, results: res, totalPages: total, page: 1, err: err}
				}
			} else if !v.input.Focused() && len(v.results) > 0 && v.cursor < len(v.results) {
				selectedID := v.results[v.cursor].ID
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

		case "[", "p":
			if !v.input.Focused() && v.page > 1 {
				v.page--
				query := strings.TrimSpace(v.input.Value())
				return v, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					if v.src == nil {
						return searchResultMsg{query: query, results: nil, totalPages: 1, page: v.page, err: nil}
					}
					res, total, err := v.src.Search(ctx, source.SearchTypeTitle, query, v.page)
					return searchResultMsg{query: query, results: res, totalPages: total, page: v.page, err: err}
				}
			}
		case "]", "n":
			if !v.input.Focused() && v.page < v.totalPages {
				v.page++
				query := strings.TrimSpace(v.input.Value())
				return v, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					if v.src == nil {
						return searchResultMsg{query: query, results: nil, totalPages: 1, page: v.page, err: nil}
					}
					res, total, err := v.src.Search(ctx, source.SearchTypeTitle, query, v.page)
					return searchResultMsg{query: query, results: res, totalPages: total, page: v.page, err: err}
				}
			}

		case "down", "ctrl+j":
			if v.input.Focused() && len(v.results) > 0 {
				v.input.Blur()
			} else if len(v.results) > 0 && v.cursor < len(v.results)-1 {
				v.cursor++
				v.adjustOffset()
			}
		case "up", "ctrl+k":
			if !v.input.Focused() && len(v.results) > 0 {
				if v.cursor > 0 {
					v.cursor--
					v.adjustOffset()
				} else {
					v.input.Focus()
				}
			}
		case "pgup", "ctrl+u", "b":
			if !v.input.Focused() && len(v.results) > 0 {
				v.cursor -= visible
				if v.cursor < 0 {
					v.cursor = 0
				}
				v.adjustOffset()
			}
		case "pgdown", "ctrl+d", "f":
			if !v.input.Focused() && len(v.results) > 0 {
				v.cursor += visible
				if v.cursor >= len(v.results) {
					v.cursor = len(v.results) - 1
				}
				v.adjustOffset()
			}
		case "g", "home":
			if !v.input.Focused() && len(v.results) > 0 {
				v.cursor = 0
				v.offset = 0
			}
		case "G", "end":
			if !v.input.Focused() && len(v.results) > 0 {
				v.cursor = len(v.results) - 1
				v.adjustOffset()
			}
		case "d", "c":
			if !v.input.Focused() && len(v.results) > 0 && v.cursor < len(v.results) {
				if v.onDownload != nil {
					return v, v.onDownload(v.results[v.cursor].ID)
				}
			}
		case "e":
			if !v.input.Focused() && len(v.results) > 0 && v.cursor < len(v.results) {
				if v.onExport != nil {
					return v, v.onExport(v.results[v.cursor].ID, common.ExportModeFullBook)
				}
			}
		case "s":
			if !v.input.Focused() && len(v.results) > 0 && v.cursor < len(v.results) {
				if v.onExport != nil {
					return v, v.onExport(v.results[v.cursor].ID, common.ExportModeAllVolumesSeparate)
				}
			}
		case "/":
			if !v.input.Focused() {
				v.input.Focus()
				return v, nil
			}
		case "esc":
			if v.input.Focused() && len(v.results) > 0 {
				v.input.Blur()
				return v, nil
			}
		}
	}

	var inputCmd tea.Cmd
	v.input, inputCmd = v.input.Update(msg)
	cmds = append(cmds, inputCmd)

	return v, tea.Batch(cmds...)
}

// View renders SearchView.
func (v *SearchView) View() string {
	var sb strings.Builder

	header := lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
		Render("在线轻小说检索 (Wenku8)")
	sb.WriteString(header + "\n")

	maxWidth := v.width - 2
	if maxWidth < 30 {
		maxWidth = 30
	}

	// Line 2: Tips
	var tipsText string
	if maxWidth >= 75 {
		tipsText = " 检索提示: 输入书名或作者名检索轻小说 (按 [Enter] 开始检索，按 [↓] 浏览结果)"
	} else {
		tipsText = " 检索提示: 输入书名或作者名检索 (按 [Enter] 检索)"
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentSky).
		Render(theme.TruncateANSI(tipsText, maxWidth, "...")) + "\n")

	// Line 3: Input bar
	inputBar := lipgloss.NewStyle().
		Background(theme.BarBg).
		Width(maxWidth).
		Render(v.input.View())
	sb.WriteString(inputBar + "\n")

	if v.searching {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentSky).
			Render(" [检索中] 正在联网检索轻小说，请稍候...") + "\n")
		return sb.String()
	}

	if v.err != nil {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentRose).
			Render(fmt.Sprintf(" [错误] 检索出错: %v", v.err)) + "\n")
		return sb.String()
	}

	if len(v.results) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.TextDim).
			Render(" [提示] 请在上方输入关键词检索轻小说，按 [Enter] 开始检索。") + "\n")
		return sb.String()
	}

	// Line 4: Stats & action hint
	curPos := v.cursor + 1
	focusHint := "[d] 下载全本  •  [/] 输入框"
	if v.input.Focused() {
		focusHint = "[Enter] 检索  •  [↓] 结果列表"
	}
	pageStr := ""
	if v.totalPages > 1 {
		pageStr = fmt.Sprintf("第 %d/%d 页 ([/])  •  ", v.page, v.totalPages)
	}
	statsText := fmt.Sprintf(" %s共 %d 本  •  当前 [%d/%d]  •  [Enter] 目录  •  %s",
		pageStr, len(v.results), curPos, len(v.results), focusHint)
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.PrimaryLight).
		Render(theme.TruncateANSI(statsText, maxWidth, "...")) + "\n")

	v.adjustOffset()
	visible := v.visibleCards()
	start := v.offset
	end := start + visible
	if end > len(v.results) {
		end = len(v.results)
	}

	// Line 5: Top fold indicator (strictly 1 line)
	if start > 0 {
		msg := fmt.Sprintf("  ▲ 上方还有 %d 部小说已折叠 (向上滚动查看) ", start)
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

	const (
		barActive       = "▎ ▶ "
		barActiveIndent = "▎   "
		barInactive     = "│   "
	)

	for i := start; i < end; i++ {
		b := v.results[i]
		isSelected := i == v.cursor && !v.input.Focused()

		var line1Badges string
		if b.IsComplete {
			line1Badges = theme.BadgeSuccess.Render("完结")
		}
		if b.IsBlocked {
			if line1Badges != "" {
				line1Badges += " "
			}
			line1Badges += theme.BadgeWarning.Render("版权受限")
		}

		desc := theme.CleanDescription(b.Description)
		if desc == "" {
			desc = "暂无简介"
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
		if b.IsBlocked {
			statusStr += " (版权受限)"
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
			if theme.StringWidth(b.Title) > titleBudget {
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
			if theme.StringWidth(b.Title) > titleBudget {
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

	// Line N: Bottom fold indicator (strictly 1 line)
	if end < len(v.results) {
		remaining := len(v.results) - end
		msg := fmt.Sprintf("  ▼ 下方还有 %d 部小说已折叠 (向下滚动查看) ", remaining)
		ruleLen := maxWidth - theme.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentAmber).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	} else {
		msg := fmt.Sprintf("  [全部] 已显示全部 %d 部搜索结果 ", len(v.results))
		ruleLen := maxWidth - theme.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentEmerald).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	}

	return sb.String()
}
