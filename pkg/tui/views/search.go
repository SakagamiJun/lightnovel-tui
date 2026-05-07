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
	// Each search item takes exactly 2 lines.
	avail := v.height - 6
	if avail < 2 {
		return 1
	}
	cards := avail / 2
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
	focusHint := "[/ 或 Esc] 输入关键词"
	if v.input.Focused() {
		focusHint = "[Enter] 搜索, [↓] 选择结果"
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentColor).
		Render(fmt.Sprintf("共检索到 %d 条结果 (%d/%d) - [↑/↓/滚轮] 选择, [Enter] 查看目录, %s",
			len(v.results), curPos, len(v.results), focusHint)) + "\n")

	v.adjustOffset()
	visible := v.visibleCards()
	start := v.offset
	end := start + visible
	if end > len(v.results) {
		end = len(v.results)
	}

	maxWidth := v.width - 2
	if maxWidth < 30 {
		maxWidth = 30
	}

	// Top fold indicator (strictly 1 line)
	if start > 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render(fmt.Sprintf("  ▲ 上方还有 %d 条结果 (向上滚动查看)", start)) + "\n")
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.BorderColor).
			Render(strings.Repeat("─", maxWidth)) + "\n")
	}

	for i := start; i < end; i++ {
		b := v.results[i]
		isSelected := i == v.cursor && !v.input.Focused()

		indicator := "  "
		if isSelected {
			indicator = "▶ "
		}

		desc := strings.ReplaceAll(b.Description, "\r", " ")
		desc = strings.ReplaceAll(desc, "\n", " ")
		desc = strings.TrimSpace(desc)
		if desc == "" {
			desc = "暂无简介"
		}

		line1Raw := fmt.Sprintf("%s%s (ID: %s)  %s · %s · %d字",
			indicator, b.Title, b.ID, b.Author, b.Publisher, b.WordCount)
		line2Raw := fmt.Sprintf("    简介: %s", desc)

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

	// Bottom fold indicator (strictly 1 line)
	if end < len(v.results) {
		remaining := len(v.results) - end
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render(fmt.Sprintf("  ▼ 下方还有 %d 条结果 (向下滚动查看)", remaining)) + "\n")
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.MutedColor).
			Render("  ✓ 已显示到底部") + "\n")
	}

	return sb.String()
}
