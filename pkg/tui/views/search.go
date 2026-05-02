package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	src       source.DataSource
	input     textinput.Model
	results   []model.BookSummary
	cursor    int
	offset    int
	searching bool
	err       error
	width     int
	height    int
	onSelect  func(bookID string) tea.Cmd
}

// NewSearchView creates an interactive search view.
func NewSearchView(src source.DataSource, onSelect func(bookID string) tea.Cmd) *SearchView {
	ti := textinput.New()
	ti.Placeholder = "输入书名或作者 (回车开始检索)..."
	ti.Focus()
	ti.CharLimit = 50
	ti.Width = 40

	return &SearchView{
		src:      src,
		input:    ti,
		results:  make([]model.BookSummary, 0),
		onSelect: onSelect,
	}
}

func (v *SearchView) Init() tea.Cmd {
	return textinput.Blink
}

func (v *SearchView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

func (v *SearchView) visibleCards() int {
	// Header + input + instructions = ~5 lines. Each search card + margin = 4 lines.
	cards := (v.height - 7) / 4
	if cards < 2 {
		cards = 2
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
		return v, nil

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if len(v.results) > 0 && v.cursor > 0 {
				v.cursor--
				v.adjustOffset()
			}
		case tea.MouseButtonWheelDown:
			if len(v.results) > 0 && v.cursor < len(v.results)-1 {
				v.cursor++
				v.adjustOffset()
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
			} else if len(v.results) > 0 && v.cursor < len(v.results) {
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

		case "up", "ctrl+k":
			if len(v.results) > 0 && v.cursor > 0 {
				v.cursor--
				v.adjustOffset()
			}
		case "down", "ctrl+j":
			if len(v.results) > 0 && v.cursor < len(v.results)-1 {
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
			if v.cursor >= len(v.results) {
				v.cursor = len(v.results) - 1
			}
			v.adjustOffset()
		case "esc":
			v.input.Focus()
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
		Render("🔍 在线小说检索 (Wenku8)")
	sb.WriteString(header + "\n")
	sb.WriteString(v.input.View() + "\n\n")

	if v.searching {
		sb.WriteString("⏳ 正在网络检索中，请稍候...\n")
		return sb.String()
	}

	if v.err != nil {
		sb.WriteString(fmt.Sprintf("❌ 检索出错: %v\n", v.err))
		return sb.String()
	}

	if len(v.results) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render("请输入关键词并按 [Enter] 开始搜索。"))
		return sb.String()
	}

	curPos := v.cursor + 1
	sb.WriteString(fmt.Sprintf("共检索到 %d 条结果 (%d/%d) - [↑/↓/滚轮] 选择, [PgUp/PgDn] 翻页, [Enter] 查看目录/阅读:\n",
		len(v.results), curPos, len(v.results)))

	v.adjustOffset()
	visible := v.visibleCards()
	start := v.offset
	end := start + visible
	if end > len(v.results) {
		end = len(v.results)
	}

	cardWidth := v.width - 6
	if cardWidth < 30 {
		cardWidth = 30
	}

	if start > 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render(fmt.Sprintf("  ▲ 上方还有 %d 条结果被折叠 (向上滚动查看)", start)) + "\n")
	} else {
		sb.WriteString("\n")
	}

	for i := start; i < end; i++ {
		b := v.results[i]
		isSelected := i == v.cursor
		style := theme.CardStyle
		indicator := "  "
		if isSelected {
			style = theme.CardActiveStyle
			indicator = "▶ "
		}

		desc := b.Description
		if len([]rune(desc)) > 50 {
			desc = string([]rune(desc)[:50]) + "..."
		}

		content := fmt.Sprintf("%s%s (ID: %s)\n作者: %s | 文库: %s | %d 字\n简介: %s",
			indicator, b.Title, b.ID, b.Author, b.Publisher, b.WordCount, desc)

		sb.WriteString(style.Width(cardWidth).Render(content))
		sb.WriteString("\n")
	}

	if end < len(v.results) {
		remaining := len(v.results) - end
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render(fmt.Sprintf("  ▼ 下方还有 %d 条结果被折叠 (向下滚动查看)", remaining)) + "\n")
	}

	return sb.String()
}
