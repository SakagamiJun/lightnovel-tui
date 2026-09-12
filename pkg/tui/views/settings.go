package views

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/SakagamiJun/lightnovel-tui/pkg/storage"
	"github.com/SakagamiJun/lightnovel-tui/pkg/text"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/common"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/theme"
	"github.com/SakagamiJun/lightnovel-tui/pkg/version"
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
	store              *storage.Storage
	width              int
	height             int
	cursor             int
	cacheSize          int64
	sizeLoaded         bool
	breakdown          *storage.StorageBreakdown
	scrollStep         int
	autoBookmark       bool
	confirmClear       bool
	confirmCleanImages bool
	confirmCleanEpubs  bool
	exportDir          string
	cacheDir           string
	editingPath        bool
	editingItem        int // 0 for cacheDir, 1 for exportDir
	pathInput          textinput.Model
	showingRules       bool
	rulesCursor        int
	rules              []text.FormattingRule
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

	var initialRules []text.FormattingRule
	if store != nil {
		initialRules, _ = store.LoadRules()
	} else {
		initialRules = text.DefaultRules
	}

	v := &SettingsView{
		store:        store,
		cursor:       0,
		scrollStep:   3,
		autoBookmark: true,
		exportDir:    defaultExportDir,
		cacheDir:     defaultCacheDir,
		pathInput:    ti,
		rules:        initialRules,
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

// RecalculateCacheSizeCmd returns a BubbleTea Cmd to asynchronously compute cache size and breakdown.
func (v *SettingsView) RecalculateCacheSizeCmd() tea.Cmd {
	return func() tea.Msg {
		if v.store == nil {
			return storageBreakdownMsg{nil}
		}
		bd, err := v.store.GetStorageBreakdown(v.exportDir)
		if err != nil {
			sz, _ := v.store.CalculateCacheSize()
			return cacheSizeMsg(sz)
		}
		return storageBreakdownMsg{bd}
	}
}

type storageBreakdownMsg struct {
	breakdown *storage.StorageBreakdown
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

// IsShowingRules returns true if currently in rules management mode.
func (v *SettingsView) IsShowingRules() bool {
	return v.showingRules
}

// CloseRules dismisses the rules management mode and returns to settings.
func (v *SettingsView) CloseRules() {
	v.showingRules = false
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
	case storageBreakdownMsg:
		v.breakdown = msg.breakdown
		if msg.breakdown != nil {
			v.cacheSize = msg.breakdown.TotalBytes
		}
		v.sizeLoaded = true
		return v, nil

	case cacheSizeMsg:
		v.cacheSize = int64(msg)
		v.sizeLoaded = true
		return v, nil

	case tea.KeyMsg:
		// 0. If actively managing formatting rules
		if v.showingRules {
			switch msg.String() {
			case "up", "k":
				if v.rulesCursor > 0 {
					v.rulesCursor--
				}
				return v, nil
			case "down", "j":
				if v.rulesCursor < len(v.rules)-1 {
					v.rulesCursor++
				}
				return v, nil
			case "enter", " ", "space", "t":
				if len(v.rules) > 0 && v.rulesCursor < len(v.rules) {
					ruleID := v.rules[v.rulesCursor].ID
					var enabled bool
					if v.store != nil {
						var err error
						enabled, err = v.store.ToggleRule(ruleID)
						if err != nil {
							return v, func() tea.Msg { return common.StatusMsg("[错误] 切换规则失败: " + err.Error()) }
						}
					} else {
						enabled = !v.rules[v.rulesCursor].Enabled
					}
					v.rules[v.rulesCursor].Enabled = enabled
					state := "已启用"
					if !enabled {
						state = "已禁用"
					}
					return v, func() tea.Msg {
						return common.StatusMsg(fmt.Sprintf("规则《%s》%s", v.rules[v.rulesCursor].Name, state))
					}
				}
				return v, nil
			case "esc":
				v.showingRules = false
				return v, nil
			default:
				return v, nil
			}
		}

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

		// 2. If confirming clean images
		if v.confirmCleanImages {
			switch msg.String() {
			case "y", "Y":
				v.confirmCleanImages = false
				var freed int64
				if v.store != nil {
					freed, _ = v.store.CleanImagesOnly(true)
				}
				return v, tea.Batch(
					v.RecalculateCacheSizeCmd(),
					func() tea.Msg {
						return common.StatusMsg(fmt.Sprintf("[完成] 已成功清理插图缓存，共释放空间: %s (书籍封面已保留)", formatBytes(freed)))
					},
				)
			case "n", "N", "esc":
				v.confirmCleanImages = false
				return v, func() tea.Msg { return common.StatusMsg("[提示] 已取消清理插图操作") }
			}
			return v, nil
		}

		// 3. If confirming clean epubs
		if v.confirmCleanEpubs {
			switch msg.String() {
			case "y", "Y":
				v.confirmCleanEpubs = false
				var freed int64
				if v.store != nil {
					freed, _ = v.store.CleanEpubsOnly(v.exportDir)
				}
				return v, tea.Batch(
					v.RecalculateCacheSizeCmd(),
					func() tea.Msg {
						return common.StatusMsg(fmt.Sprintf("[完成] 已成功清理导出目录中的 EPUB 文件，共释放空间: %s", formatBytes(freed)))
					},
				)
			case "n", "N", "esc":
				v.confirmCleanEpubs = false
				return v, func() tea.Msg { return common.StatusMsg("[提示] 已取消清理导出文件") }
			}
			return v, nil
		}

		// 4. If confirming clear all cache
		if v.confirmClear {
			switch msg.String() {
			case "y", "Y":
				v.confirmClear = false
				if v.store != nil {
					_ = v.store.ClearCache()
					_, _ = v.store.CleanEpubsOnly(v.exportDir)
				}
				v.cacheSize = 0
				return v, tea.Batch(
					v.RecalculateCacheSizeCmd(),
					func() tea.Msg {
						return common.StatusMsg("[完成] 已成功清空全部本地小说缓存与导出文件")
					},
				)
			case "n", "N", "esc":
				v.confirmClear = false
				return v, func() tea.Msg { return common.StatusMsg("[提示] 已取消清空操作") }
			}
			return v, nil
		}

		// 5. Normal navigation & toggle
		switch msg.String() {
		case "up", "k":
			if v.cursor > 0 {
				v.cursor--
			}
		case "down", "j":
			if v.cursor < 9 {
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

			case 4: // Storage breakdown refresh & top info
				if v.breakdown != nil && len(v.breakdown.BookItems) > 0 {
					top1 := v.breakdown.BookItems[0]
					return v, tea.Batch(
						v.RecalculateCacheSizeCmd(),
						func() tea.Msg {
							return common.StatusMsg(fmt.Sprintf("[存储排行第一] %s (总计:%s | 插图:%s | 文本:%s)",
								top1.Title, formatBytes(top1.TotalBytes), formatBytes(top1.ImageBytes), formatBytes(top1.TextBytes)))
						},
					)
				}
				return v, v.RecalculateCacheSizeCmd()

			case 5: // Clean images only
				v.confirmCleanImages = true
				return v, func() tea.Msg {
					return common.StatusMsg("[清理确认] 是否仅清理插图缓存 (保留书籍封面与正文)？按键盘 [y] 确认 / 按 [n/Esc] 取消")
				}

			case 6: // Clean epubs only
				v.confirmCleanEpubs = true
				return v, func() tea.Msg {
					return common.StatusMsg("[清理确认] 是否清理导出目录下的所有 EPUB 文件？按键盘 [y] 确认 / 按 [n/Esc] 取消")
				}

			case 7: // Clear all cache
				v.confirmClear = true
				return v, func() tea.Msg {
					return common.StatusMsg("[清空确认] 再次确认：请按键盘 [y] 确认执行清空全部缓存，按 [n/Esc] 取消")
				}

			case 8: // Manage rules
				if v.store != nil {
					rules, _ := v.store.LoadRules()
					v.rules = rules
				} else {
					v.rules = text.DefaultRules
				}
				v.rulesCursor = 0
				v.showingRules = true
				return v, nil

			case 9: // Program version and check update
				info := version.GetBuildInfo()
				currentVer := version.GetVersion()
				return v, tea.Batch(
					func() tea.Msg {
						return common.StatusMsg(fmt.Sprintf("[版本] %s (正在检测更新...)", info))
					},
					func() tea.Msg {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						rel, err := version.CheckLatestRelease(ctx)
						if err != nil {
							return common.StatusMsg(fmt.Sprintf("[版本] %s (检测更新失败: %v)", info, err))
						}
						if rel.HasUpdate {
							return common.StatusMsg(fmt.Sprintf("[更新提示] 发现新版本 %s (当前: %s)，可通过 'brew upgrade lnr' 获取更新", rel.TagName, currentVer))
						}
						return common.StatusMsg(fmt.Sprintf("[版本] 当前已是最新版本 (%s)", currentVer))
					},
				)
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

	if v.showingRules {
		return v.renderRulesView(maxWidth)
	}

	var sb strings.Builder

	// Header line (strictly 1 line)
	var titleText string
	var headerStyle lipgloss.Style
	if v.confirmClear {
		titleText = " [清空确认] 将删除所有已下载章节与图片！请按键盘 [y] 确认清空 / 按 [n/Esc] 取消"
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentRose)
	} else if v.confirmCleanImages {
		titleText = " [清理插图] 将删除所有内嵌插图(保留封面与正文)！请按键盘 [y] 确认 / 按 [n/Esc] 取消"
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentAmber)
	} else if v.confirmCleanEpubs {
		titleText = " [清理导出] 将删除 exports 目录下的全部 EPUB 文件！请按键盘 [y] 确认 / 按 [n/Esc] 取消"
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentAmber)
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

	bookmarkStr := "已开启"
	bookmarkBadge := theme.BadgeSuccess.Render(bookmarkStr)
	if !v.autoBookmark {
		bookmarkStr = "已关闭"
		bookmarkBadge = theme.BadgeMuted.Render(bookmarkStr)
	}

	analysisVal := "正在统计..."
	analysisBadge := theme.BadgeInfo.Render("统计中")
	if v.breakdown != nil {
		analysisVal = fmt.Sprintf("总计: %s [插图: %s | 正文: %s | 导出: %s]",
			formatBytes(v.breakdown.TotalBytes),
			formatBytes(v.breakdown.ImageBytes),
			formatBytes(v.breakdown.TextBytes),
			formatBytes(v.breakdown.EpubBytes),
		)
		analysisBadge = theme.BadgeInfo.Render(formatBytes(v.breakdown.TotalBytes))
	} else if v.sizeLoaded {
		analysisVal = formatBytes(v.cacheSize)
		analysisBadge = theme.BadgeInfo.Render(analysisVal)
	}

	imgCleanVal := "安全定向瘦身"
	if v.breakdown != nil {
		imgCleanVal = fmt.Sprintf("可释放约 %s (书籍封面仍保留)", formatBytes(v.breakdown.ImageBytes))
	}

	epubCleanVal := "清空已导出的电子书"
	if v.breakdown != nil {
		epubCleanVal = fmt.Sprintf("可释放约 %s", formatBytes(v.breakdown.EpubBytes))
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
			label: "存储空间深度分析",
			value: analysisVal,
			badge: analysisBadge,
			desc:  "细分统计正文、插图及导出 EPUB 占用，按 [Enter] 或 [r] 刷新统计",
		},
		{
			label: "定向清理插图缓存",
			value: imgCleanVal,
			badge: theme.BadgeWarning.Render("安全瘦身"),
			desc:  "仅清理插图大图(占90%+空间)，保留正文离线阅读与书籍封面，按 [Enter] 触发",
		},
		{
			label: "定向清理导出文件",
			value: epubCleanVal,
			badge: theme.BadgeWarning.Render("清理导出"),
			desc:  "仅删除导出目录中已打包的 EPUB 文件，按 [Enter] 触发确认",
		},
		{
			label: "清空全部本地缓存",
			value: "清理本地全部小说缓存与导出文件",
			badge: theme.BadgeWarning.Render("按Enter清空"),
			desc:  "按 [Enter] 触发确认，随后需按键盘 [y] 确认执行清空 / 按 [n/Esc] 取消",
		},
		{
			label: "排版规范与清洗规则",
			value: fmt.Sprintf("已配置 %d 条规则", len(v.rules)),
			badge: theme.BadgeInfo.Render("按Enter管理"),
			desc:  "按 [Enter] 查看并开关省略号/破折号/广告过滤规则",
		},
		{
			label: "程序版本与环境",
			value: version.GetBuildInfo(),
			badge: theme.BadgeInfo.Render(version.GetVersion()),
			desc:  "按 [Enter] 在线检查 GitHub 最新版本更新，查看构建哈希与架构",
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
			} else if i == 5 && v.confirmCleanImages {
				line1Text = fmt.Sprintf("%s%s  %s  %s",
					barActive,
					lipgloss.NewStyle().Bold(true).Foreground(theme.AccentAmber).Render(item.label),
					theme.BadgeWarning.Render("待确认"),
					lipgloss.NewStyle().Bold(true).Foreground(theme.AccentAmber).Render("确认清理插图？按 [y] 确认 / 按 [n/Esc] 取消"))
				line2Text = fmt.Sprintf("%s[提示] 将保留书籍封面与正文离线阅读。按 [y] 执行 / 按 [n/Esc] 放弃", barActiveIndent)
			} else if i == 6 && v.confirmCleanEpubs {
				line1Text = fmt.Sprintf("%s%s  %s  %s",
					barActive,
					lipgloss.NewStyle().Bold(true).Foreground(theme.AccentAmber).Render(item.label),
					theme.BadgeWarning.Render("待确认"),
					lipgloss.NewStyle().Bold(true).Foreground(theme.AccentAmber).Render("确认清理导出EPUB？按 [y] 确认 / 按 [n/Esc] 取消"))
				line2Text = fmt.Sprintf("%s[提示] 将删除导出目录中的 EPUB 文件。按 [y] 执行 / 按 [n/Esc] 放弃", barActiveIndent)
			} else if i == 7 && v.confirmClear {
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
				Render(theme.TruncateANSI(line1Text, maxWidth, "..."))
			line2 := lipgloss.NewStyle().Foreground(theme.TextMuted).Background(theme.HighlightBg).Width(maxWidth).
				Render(theme.TruncateANSI(line2Text, maxWidth, "..."))

			sb.WriteString(line1 + "\n")
			sb.WriteString(line2 + "\n")
		} else {
			line1Text := fmt.Sprintf("%s%s  %s  %s",
				barInactive,
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CBD5E1")).Render(item.label),
				item.badge,
				lipgloss.NewStyle().Foreground(theme.TextMuted).Render(item.value))
			line1 := lipgloss.NewStyle().Width(maxWidth).
				Render(theme.TruncateANSI(line1Text, maxWidth, "..."))

			line2Text := fmt.Sprintf("%s%s", barInactive, item.desc)
			line2 := lipgloss.NewStyle().Foreground(theme.TextDim).Width(maxWidth).
				Render(theme.TruncateANSI(line2Text, maxWidth, "..."))

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

func (v *SettingsView) renderRulesView(maxWidth int) string {
	var sb strings.Builder
	title := " [排版规范化与正则清洗规则]  •  [Space/Enter] 启用/禁用  •  [Esc] 返回设置"
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).Render(runewidth.Truncate(title, maxWidth, "...")) + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.BorderColor).Render(strings.Repeat("─", maxWidth)) + "\n")

	if len(v.rules) == 0 {
		sb.WriteString("  当前未加载任何规则。\n")
		return sb.String()
	}

	const (
		barActive       = "▎ ▶ "
		barActiveIndent = "▎   "
		barInactive     = "│   "
	)

	for i, r := range v.rules {
		isSelected := i == v.rulesCursor
		checkbox := "[ ]"
		checkStyle := lipgloss.NewStyle().Foreground(theme.MutedColor)
		if r.Enabled {
			checkbox = "[x]"
			checkStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentEmerald)
		}

		mode := "[文本]"
		if r.IsRegex {
			mode = "[正则]"
		}
		scope := "[全局]"
		if r.BookID != "" {
			scope = fmt.Sprintf("[小说: %s]", r.BookID)
		}

		if isSelected {
			line1 := fmt.Sprintf("%s%s %s  %s",
				barActive,
				checkStyle.Render(checkbox),
				lipgloss.NewStyle().Bold(true).Foreground(theme.AccentSky).Render(r.Name),
				lipgloss.NewStyle().Foreground(theme.AccentAmber).Render(mode+" "+scope))
			line2 := fmt.Sprintf("%s匹配: %s -> %q", barActiveIndent, r.Pattern, r.Replacement)
			sb.WriteString(line1 + "\n" + lipgloss.NewStyle().Foreground(theme.TextMuted).Render(line2) + "\n\n")
		} else {
			line1 := fmt.Sprintf("%s%s %s  %s",
				barInactive,
				checkStyle.Render(checkbox),
				lipgloss.NewStyle().Foreground(theme.TextWhite).Render(r.Name),
				lipgloss.NewStyle().Foreground(theme.TextMuted).Render(mode+" "+scope))
			line2 := fmt.Sprintf("%s匹配: %s -> %q", barInactive, r.Pattern, r.Replacement)
			sb.WriteString(line1 + "\n" + lipgloss.NewStyle().Foreground(theme.TextDim).Render(line2) + "\n\n")
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
