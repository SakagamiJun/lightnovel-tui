package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"lnr-core/pkg/model"
	"lnr-core/pkg/source"
	"lnr-core/pkg/tui/common"
	"lnr-core/pkg/tui/theme"
)

type searchResultMsg struct {
	results []model.BookSummary
	err     error
}

// SearchView handles interactive search with textinput.
type SearchView struct {
	src        source.DataSource
	input      textinput.Model
	results    []model.BookSummary
	cursor     int
	offset     int
	searching  bool
	err        error
	width      int
	height     int
	onSelect   func(bookID string) tea.Cmd
	onDownload func(bookID string) tea.Cmd
	onExport   func(bookID string) tea.Cmd
}

// NewSearchView creates an interactive search view.
func NewSearchView(src source.DataSource, onSelect func(bookID string) tea.Cmd) *SearchView {
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
		src:      src,
		input:    ti,
		results:  make([]model.BookSummary, 0),
		onSelect: onSelect,
	}
}

// SetOnDownload registers download callback.
func (v *SearchView) SetOnDownload(fn func(bookID string) tea.Cmd) {
	v.onDownload = fn
}

// SetOnExport registers export callback.
func (v *SearchView) SetOnExport(fn func(bookID string) tea.Cmd) {
	v.onExport = fn
}

func (v *SearchView) Init() tea.Cmd {
	return textinput.Blink
}

func (v *SearchView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// SetResults populates search results directly.
func (v *SearchView) SetResults(results []model.BookSummary) {
	v.searching = false
	v.results = results
	v.err = nil
	v.cursor = 0
	v.offset = 0
	if len(results) > 0 {
		v.input.Blur()
	}
}

func (v *SearchView) visibleCards() int {
	// Fixed lines:
	// Header (1) + Input (1) + Spacer (1) + Stats (1) + TopInd (1) + BotInd (1) = 6 lines.
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

func (v *SearchView) Update(msg tea.Msg) (*SearchView, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case searchResultMsg:
		v.searching = false
		v.results = msg.results
		v.err = msg.err
		v.cursor = 0
		v.offset = 0
		if len(msg.results) > 0 {
			v.input.Blur()
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
				query := strings.TrimSpace(v.input.Value())
				return v, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					res, _, err := v.src.Search(ctx, source.SearchTypeTitle, query, 1)
					return searchResultMsg{results: res, err: err}
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
					return v, v.onExport(v.results[v.cursor].ID)
				}
			}
		case "esc", "/":
			if !v.input.Focused() {
				v.input.Focus()
				return v, nil
			}
		}
	}

	var inputCmd tea.Cmd
	v.input, inputCmd = v.input.Update(msg)
	cmds = append(cmds, inputCmd)

	return v, tea.Batch(cmds...)
}

func (v *SearchView) View() string {
	var sb strings.Builder

	header := lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
		Render("在线轻小说检索 (Wenku8)")
	sb.WriteString(header + "\n")

	maxWidth := v.width - 2
	if maxWidth < 30 {
		maxWidth = 30
	}

	inputBar := lipgloss.NewStyle().
		Background(theme.BarBg).
		Width(maxWidth).
		Render(v.input.View())
	sb.WriteString(inputBar + "\n\n")

	if v.searching {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentSky).
			Render("[检索中] 正在联网检索，请稍候...") + "\n")
		return sb.String()
	}

	if v.err != nil {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentRose).
			Render(fmt.Sprintf("[错误] 检索出错: %v", v.err)) + "\n")
		return sb.String()
	}

	if len(v.results) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.TextDim).
			Render("提示: 请在上方输入关键词并按 [Enter] 开始搜索。") + "\n")
		return sb.String()
	}

	curPos := v.cursor + 1
	focusHint := "[d] 下载全本  •  [e] 导出EPUB  •  [/] 输入框"
	if v.input.Focused() {
		focusHint = "[Enter] 检索  •  [↓] 结果列表"
	}
	statsText := fmt.Sprintf(" 找到 %d 本小说  •  当前 [%d/%d]  •  [Enter] 目录  •  %s",
		len(v.results), curPos, len(v.results), focusHint)
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.PrimaryLight).
		Render(runewidth.Truncate(statsText, maxWidth, "...")) + "\n")

	v.adjustOffset()
	visible := v.visibleCards()
	start := v.offset
	end := start + visible
	if end > len(v.results) {
		end = len(v.results)
	}

	// Top fold indicator (strictly 1 line)
	if start > 0 {
		msg := fmt.Sprintf("  ▲ 上方还有 %d 部小说已折叠 (向上滚动查看) ", start)
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
		b := v.results[i]
		isSelected := i == v.cursor && !v.input.Focused()

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

		// Description cleanup
		desc := strings.ReplaceAll(b.Description, "\r", " ")
		desc = strings.ReplaceAll(desc, "\n", " ")
		desc = strings.TrimSpace(desc)
		if desc == "" {
			desc = "暂无简介"
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
			metaContent := fmt.Sprintf("%s作者: %s    字数: %s    文库: %s",
				barActiveIndent, b.Author, wordCountStr, b.Publisher)
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
			metaContent := fmt.Sprintf("%s作者: %s    字数: %s    文库: %s",
				barInactive, b.Author, wordCountStr, b.Publisher)
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

	// Bottom fold indicator (strictly 1 line)
	if end < len(v.results) {
		remaining := len(v.results) - end
		msg := fmt.Sprintf("  ▼ 下方还有 %d 部小说已折叠 (向下滚动查看) ", remaining)
		ruleLen := maxWidth - runewidth.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentAmber).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	} else {
		msg := fmt.Sprintf("  [全部] 已显示全部 %d 部搜索结果 ", len(v.results))
		ruleLen := maxWidth - runewidth.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentEmerald).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	}

	return sb.String()
}
