package views

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/common"
	"lnr-core/pkg/tui/theme"
)

// SettingsView provides an interactive configuration panel.
type SettingsView struct {
	store        *storage.Storage
	width        int
	height       int
	cursor       int
	cacheSize    int64
	sizeLoaded   bool
	scrollStep   int
	autoBookmark bool
	confirmClear bool
	exportDir    string
}

// NewSettingsView constructs a new SettingsView instance.
func NewSettingsView(store *storage.Storage) *SettingsView {
	home, _ := os.UserHomeDir()
	exportDir := filepath.Join(home, ".lnr", "exports")
	return &SettingsView{
		store:        store,
		cursor:       0,
		scrollStep:   3,
		autoBookmark: true,
		exportDir:    exportDir,
	}
}

// RecalculateCacheSizeCmd returns a BubbleTea Cmd to asynchronously compute cache size.
func (v *SettingsView) RecalculateCacheSizeCmd() tea.Cmd {
	return func() tea.Msg {
		if v.store == nil {
			return cacheSizeMsg(0)
		}
		sz, err := v.store.CalculateCacheSize()
		if err != nil {
			return cacheSizeMsg(0)
		}
		return cacheSizeMsg(sz)
	}
}

type cacheSizeMsg int64

func (v *SettingsView) Init() tea.Cmd {
	return v.RecalculateCacheSizeCmd()
}

func (v *SettingsView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// Cursor returns current active setting cursor index.
func (v *SettingsView) Cursor() int {
	return v.cursor
}

// ScrollStep returns reader scroll step lines.
func (v *SettingsView) ScrollStep() int {
	return v.scrollStep
}

func formatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func (v *SettingsView) Update(msg tea.Msg) (*SettingsView, tea.Cmd) {
	switch msg := msg.(type) {
	case cacheSizeMsg:
		v.cacheSize = int64(msg)
		v.sizeLoaded = true
		return v, nil

	case tea.KeyMsg:
		if v.confirmClear {
			switch msg.String() {
			case "y", "Y":
				v.confirmClear = false
				if v.store != nil {
					_ = v.store.ClearCache()
				}
				v.cacheSize = 0
				return v, tea.Batch(
					v.RecalculateCacheSizeCmd(),
					func() tea.Msg { return common.StatusMsg("已成功清空本地缓存") },
				)
			case "n", "N", "esc":
				v.confirmClear = false
				return v, func() tea.Msg { return common.StatusMsg("已取消清空操作") }
			}
			return v, nil
		}

		switch msg.String() {
		case "up", "k":
			if v.cursor > 0 {
				v.cursor--
			}
		case "down", "j":
			if v.cursor < 5 {
				v.cursor++
			}
		case "enter", " ":
			switch v.cursor {
			case 2: // Scroll step
				switch v.scrollStep {
				case 1:
					v.scrollStep = 3
				case 3:
					v.scrollStep = 5
				case 5:
					v.scrollStep = 10
				default:
					v.scrollStep = 1
				}
				return v, func() tea.Msg {
					return common.StatusMsg(fmt.Sprintf("滚轮步长已设置为 %d 行", v.scrollStep))
				}
			case 3: // Auto bookmark
				v.autoBookmark = !v.autoBookmark
				state := "开启"
				if !v.autoBookmark {
					state = "关闭"
				}
				return v, func() tea.Msg {
					return common.StatusMsg(fmt.Sprintf("阅读自动记录书签已%s", state))
				}
			case 4: // Refresh cache size
				return v, v.RecalculateCacheSizeCmd()
			case 5: // Clear cache
				v.confirmClear = true
				return v, nil
			}
		case "r":
			return v, v.RecalculateCacheSizeCmd()
		}
	}
	return v, nil
}

func (v *SettingsView) View() string {
	maxWidth := v.width - 2
	if maxWidth < 30 {
		maxWidth = 30
	}

	var sb strings.Builder

	// Header line (1 line)
	titleText := " 系统配置与偏好设置  •  [↑/↓] 导航  •  [Enter/空格] 切换/执行  •  [r] 重新统计空间"
	header := lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
		Render(runewidth.Truncate(titleText, maxWidth, "..."))
	sb.WriteString(header + "\n")

	// Divider line (1 line)
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.BorderColor).
		Render(strings.Repeat("─", maxWidth)) + "\n")

	cacheDir := "未配置"
	if v.store != nil {
		cacheDir = v.store.BaseDir()
	}

	sizeStr := "正在统计..."
	if v.sizeLoaded {
		sizeStr = formatBytes(v.cacheSize)
	}

	bookmarkStr := "已开启"
	bookmarkBadge := theme.BadgeSuccess.Render(bookmarkStr)
	if !v.autoBookmark {
		bookmarkStr = "已关闭"
		bookmarkBadge = theme.BadgeMuted.Render(bookmarkStr)
	}

	type settingItem struct {
		label string
		value string
		badge string
		desc  string
	}

	items := []settingItem{
		{
			label: "本地缓存目录",
			value: cacheDir,
			badge: theme.BadgeInfo.Render("路径"),
			desc:  "所有已下载的书籍详情、目录、章节文本及插图存储路径",
		},
		{
			label: "EPUB 导出目录",
			value: v.exportDir,
			badge: theme.BadgeInfo.Render("路径"),
			desc:  "导出的 EPUB 电子书文件默认保存位置",
		},
		{
			label: "沉浸阅读步长",
			value: fmt.Sprintf("%d 行", v.scrollStep),
			badge: theme.BadgeWarning.Render(fmt.Sprintf("%d 行", v.scrollStep)),
			desc:  "按 [Enter] 轮转阅读界面的滚轮翻页步长 (1 / 3 / 5 / 10 行)",
		},
		{
			label: "自动保存书签",
			value: bookmarkStr,
			badge: bookmarkBadge,
			desc:  "按 [Enter] 切换是否在阅读时自动记录上次浏览的章节位置",
		},
		{
			label: "缓存空间统计",
			value: sizeStr,
			badge: theme.BadgeInfo.Render(sizeStr),
			desc:  "当前已下载轻小说占用的本地磁盘空间，按 [Enter] 或 [r] 刷新统计",
		},
		{
			label: "清空全部缓存",
			value: "清理本地全部小说缓存",
			badge: theme.BadgeWarning.Render("管理"),
			desc:  "按 [Enter] 清理已下载书籍正文及图片缓存文件 (保留基础配置)",
		},
	}

	const (
		barActive       = "▎ ▶ "
		barActiveIndent = "▎   "
		barInactive     = "│   "
	)

	// Render items (each 2 lines: line1 = label+badge+value, line2 = desc)
	for i, item := range items {
		isSelected := i == v.cursor

		if isSelected {
			var line1Text string
			if i == 5 && v.confirmClear {
				line1Text = fmt.Sprintf("%s%s  %s  %s",
					barActive,
					lipgloss.NewStyle().Bold(true).Foreground(theme.AccentRose).Render(item.label),
					theme.BadgeWarning.Render("待确认"),
					lipgloss.NewStyle().Bold(true).Foreground(theme.AccentRose).Render("确认清空全部本地小说？[y] 确认 / [n/Esc] 取消"))
			} else {
				line1Text = fmt.Sprintf("%s%s  %s  %s",
					barActive,
					lipgloss.NewStyle().Bold(true).Foreground(theme.TextWhite).Render(item.label),
					item.badge,
					lipgloss.NewStyle().Foreground(theme.PrimaryLight).Render(item.value))
			}
			line1 := lipgloss.NewStyle().Background(theme.HighlightBg).Width(maxWidth).
				Render(runewidth.Truncate(line1Text, maxWidth, "..."))

			line2Text := fmt.Sprintf("%s%s", barActiveIndent, item.desc)
			line2 := lipgloss.NewStyle().Foreground(theme.TextMuted).Background(theme.HighlightBg).Width(maxWidth).
				Render(runewidth.Truncate(line2Text, maxWidth, "..."))

			sb.WriteString(line1 + "\n")
			sb.WriteString(line2 + "\n")
		} else {
			line1Text := fmt.Sprintf("%s%s  %s  %s",
				barInactive,
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CBD5E1")).Render(item.label),
				item.badge,
				lipgloss.NewStyle().Foreground(theme.TextMuted).Render(item.value))
			line1 := lipgloss.NewStyle().Width(maxWidth).
				Render(runewidth.Truncate(line1Text, maxWidth, "..."))

			line2Text := fmt.Sprintf("%s%s", barInactive, item.desc)
			line2 := lipgloss.NewStyle().Foreground(theme.TextDim).Width(maxWidth).
				Render(runewidth.Truncate(line2Text, maxWidth, "..."))

			sb.WriteString(line1 + "\n")
			sb.WriteString(line2 + "\n")
		}
	}

	content := sb.String()
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if len(lines) > v.height {
		lines = lines[:v.height]
	}
	for len(lines) < v.height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
