package views

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/common"
	"lnr-core/pkg/tui/theme"
)

// AppConfig represents persisted user preferences.
type AppConfig struct {
	CacheDir     string `json:"cache_dir,omitempty"`
	ExportDir    string `json:"export_dir,omitempty"`
	ScrollStep   int    `json:"scroll_step,omitempty"`
	AutoBookmark bool   `json:"auto_bookmark"`
}

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
	cacheDir     string
	editingPath  bool
	editingItem  int // 0 for cacheDir, 1 for exportDir
	pathInput    textinput.Model
}

func (v *SettingsView) configFilePath() string {
	if v.store != nil {
		return filepath.Join(v.store.BaseDir(), "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "./config.json"
	}
	return filepath.Join(home, ".lnr", "config.json")
}

// NewSettingsView constructs a new SettingsView instance.
func NewSettingsView(store *storage.Storage) *SettingsView {
	home, _ := os.UserHomeDir()
	defaultExportDir := filepath.Join(home, ".lnr", "exports")
	defaultCacheDir := ""
	if store != nil {
		defaultCacheDir = store.BaseDir()
	} else {
		defaultCacheDir = filepath.Join(home, ".lnr", "cache")
	}

	ti := textinput.New()
	ti.CharLimit = 200
	ti.Width = 60
	ti.Prompt = " "
	ti.TextStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.TextWhite)

	v := &SettingsView{
		store:        store,
		cursor:       0,
		scrollStep:   3,
		autoBookmark: true,
		exportDir:    defaultExportDir,
		cacheDir:     defaultCacheDir,
		pathInput:    ti,
	}

	// Load existing persisted configuration if present
	v.loadConfig()

	return v
}

func (v *SettingsView) loadConfig() {
	data, err := os.ReadFile(v.configFilePath())
	if err != nil {
		return
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err == nil {
		if cfg.CacheDir != "" {
			v.cacheDir = cfg.CacheDir
			if v.store != nil {
				_ = v.store.SetBaseDir(cfg.CacheDir)
			}
		}
		if cfg.ExportDir != "" {
			v.exportDir = cfg.ExportDir
		}
		if cfg.ScrollStep > 0 {
			v.scrollStep = cfg.ScrollStep
		}
		v.autoBookmark = cfg.AutoBookmark
	}
}

func (v *SettingsView) saveConfig() {
	cfg := AppConfig{
		CacheDir:     v.cacheDir,
		ExportDir:    v.exportDir,
		ScrollStep:   v.scrollStep,
		AutoBookmark: v.autoBookmark,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	cfgPath := v.configFilePath()
	_ = os.MkdirAll(filepath.Dir(cfgPath), 0755)
	_ = os.WriteFile(cfgPath, data, 0644)
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

// ExportDir returns the configured EPUB export directory.
func (v *SettingsView) ExportDir() string {
	return v.exportDir
}

// CacheDir returns the configured local cache directory.
func (v *SettingsView) CacheDir() string {
	return v.cacheDir
}

// IsEditing returns true if currently in path input editing mode.
func (v *SettingsView) IsEditing() bool {
	return v.editingPath
}

// IsConfirmingClear returns true if currently in clear cache confirmation mode.
func (v *SettingsView) IsConfirmingClear() bool {
	return v.confirmClear
}

// PathInputValue returns current value in the path input box.
func (v *SettingsView) PathInputValue() string {
	return v.pathInput.Value()
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

func expandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}

func (v *SettingsView) Update(msg tea.Msg) (*SettingsView, tea.Cmd) {
	switch msg := msg.(type) {
	case cacheSizeMsg:
		v.cacheSize = int64(msg)
		v.sizeLoaded = true
		return v, nil

	case tea.KeyMsg:
		// 1. If actively editing path for Item 0 (CacheDir) or Item 1 (ExportDir)
		if v.editingPath {
			switch msg.String() {
			case "enter":
				rawPath := strings.TrimSpace(v.pathInput.Value())
				newPath := expandHome(rawPath)
				if newPath != "" {
					if v.editingItem == 0 {
						if v.store != nil {
							if err := v.store.SetBaseDir(newPath); err != nil {
								return v, func() tea.Msg { return common.ErrorMsg(err) }
							}
						}
						v.cacheDir = newPath
						v.saveConfig()
						v.editingPath = false
						v.pathInput.Blur()
						return v, tea.Batch(
							v.RecalculateCacheSizeCmd(),
							func() tea.Msg { return common.StatusMsg("[成功] 缓存目录已修改为: " + newPath) },
						)
					} else if v.editingItem == 1 {
						_ = os.MkdirAll(newPath, 0755)
						v.exportDir = newPath
						v.saveConfig()
						v.editingPath = false
						v.pathInput.Blur()
						return v, func() tea.Msg { return common.StatusMsg("[成功] EPUB导出目录已修改为: " + newPath) }
					}
				}
				v.editingPath = false
				v.pathInput.Blur()
				return v, nil

			case "ctrl+u":
				v.pathInput.Reset()
				return v, nil

			case "esc":
				v.editingPath = false
				v.pathInput.Blur()
				return v, func() tea.Msg { return common.StatusMsg("[提示] 已取消修改路径") }

			default:
				var cmd tea.Cmd
				v.pathInput, cmd = v.pathInput.Update(msg)
				return v, cmd
			}
		}

		// 2. If confirming clear cache
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
					func() tea.Msg { return common.StatusMsg("[完成] 已成功清空全部本地小说缓存") },
				)
			case "n", "N", "esc":
				v.confirmClear = false
				return v, func() tea.Msg { return common.StatusMsg("[提示] 已取消清空操作") }
			}
			return v, nil
		}

		// 3. Normal navigation & toggle
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
			case 0: // Edit Cache Dir
				v.editingPath = true
				v.editingItem = 0
				v.pathInput.SetValue(v.cacheDir)
				v.pathInput.Focus()
				return v, textinput.Blink

			case 1: // Edit Export Dir
				v.editingPath = true
				v.editingItem = 1
				v.pathInput.SetValue(v.exportDir)
				v.pathInput.Focus()
				return v, textinput.Blink

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
				v.saveConfig()
				return v, func() tea.Msg {
					return common.StatusMsg(fmt.Sprintf("滚轮步长已设置为 %d 行", v.scrollStep))
				}

			case 3: // Auto bookmark
				v.autoBookmark = !v.autoBookmark
				v.saveConfig()
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
				return v, func() tea.Msg {
					return common.StatusMsg("[清空确认] 再次确认：请按键盘 [y] 确认执行清空，按 [n/Esc] 取消")
				}
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

	// Header line (strictly 1 line)
	var titleText string
	var headerStyle lipgloss.Style
	if v.confirmClear {
		titleText = " [清空确认] 将删除所有已下载章节与图片！请按键盘 [y] 确认清空 / 按 [n/Esc] 取消"
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentRose)
	} else if v.editingPath {
		titleText = " [编辑路径] 请输入新路径，按 [Enter] 确认保存，按 [Esc] 取消修改"
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentSky)
	} else {
		titleText = " 系统配置与偏好设置  •  [Enter] 修改/切换/执行  •  [r] 重新统计空间"
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor)
	}
	sb.WriteString(headerStyle.Render(runewidth.Truncate(titleText, maxWidth, "...")) + "\n")

	// Divider line (1 line)
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.BorderColor).
		Render(strings.Repeat("─", maxWidth)) + "\n")

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
			value: v.cacheDir,
			badge: theme.BadgeInfo.Render("按Enter修改"),
			desc:  "按 [Enter] 编辑修改缓存存储路径，按 [Esc] 取消",
		},
		{
			label: "EPUB 导出目录",
			value: v.exportDir,
			badge: theme.BadgeInfo.Render("按Enter修改"),
			desc:  "按 [Enter] 编辑修改 EPUB 导出文件保存路径，按 [Esc] 取消",
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
			badge: theme.BadgeWarning.Render("按Enter清空"),
			desc:  "按 [Enter] 触发确认，随后需按键盘 [y] 确认执行清空 / 按 [n/Esc] 取消",
		},
	}

	const (
		barActive       = "▎ ▶ "
		barActiveIndent = "▎   "
		barInactive     = "│   "
	)

	// Render items (each item strictly 2 lines)
	for i, item := range items {
		isSelected := i == v.cursor

		if isSelected {
			var line1Text string
			var line2Text string

			if (i == 0 || i == 1) && v.editingPath && v.editingItem == i {
				// Inline text input editing mode
				line1Text = fmt.Sprintf("%s%s  %s  %s",
					barActive,
					lipgloss.NewStyle().Bold(true).Foreground(theme.AccentSky).Render(item.label),
					theme.BadgeWarning.Render("编辑中"),
					lipgloss.NewStyle().Foreground(theme.PrimaryLight).Render("[Enter] 保存 / [Esc] 取消"))
				line2Text = fmt.Sprintf("%s新路径: %s", barActiveIndent, v.pathInput.View())
			} else if i == 5 && v.confirmClear {
				// Clear confirmation mode
				line1Text = fmt.Sprintf("%s%s  %s  %s",
					barActive,
					lipgloss.NewStyle().Bold(true).Foreground(theme.AccentRose).Render(item.label),
					theme.BadgeWarning.Render("待确认"),
					lipgloss.NewStyle().Bold(true).Foreground(theme.AccentRose).Render("确认清空？按 [y] 确认 / 按 [n/Esc] 取消"))
				line2Text = fmt.Sprintf("%s[警告] 该操作不可逆！将删除所有本地缓存小说。按 [y] 确认执行 / 按 [n/Esc] 放弃", barActiveIndent)
			} else {
				// Normal selected item
				line1Text = fmt.Sprintf("%s%s  %s  %s",
					barActive,
					lipgloss.NewStyle().Bold(true).Foreground(theme.TextWhite).Render(item.label),
					item.badge,
					lipgloss.NewStyle().Foreground(theme.PrimaryLight).Render(item.value))
				line2Text = fmt.Sprintf("%s%s", barActiveIndent, item.desc)
			}

			line1 := lipgloss.NewStyle().Background(theme.HighlightBg).Width(maxWidth).
				Render(runewidth.Truncate(line1Text, maxWidth, "..."))
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
