package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"lnr-core/pkg/model"
	"lnr-core/pkg/source"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/common"
	"lnr-core/pkg/tui/theme"
)

// ExploreSubTab defines the active subcategory in ExploreView.
type ExploreSubTab int

const (
	SubTabHot ExploreSubTab = iota
	SubTabAnime
	SubTabUpdate
	SubTabPostDate
	SubTabCompleted
	SubTabTags
)

var defaultExploreTags = []string{
	"校园", "青春", "恋爱", "治愈", "群像",
	"竞技", "音乐", "美食", "旅行", "欢乐向",
	"经营", "职场", "斗智", "脑洞", "宅文化",
	"穿越", "奇幻", "魔法", "异能", "战斗",
	"科幻", "机战", "战争", "冒险", "龙傲天",
	"悬疑", "犯罪", "复仇", "黑暗", "猎奇",
	"惊悚", "间谍", "末日", "游戏", "大逃杀",
	"青梅竹马", "妹妹", "女儿", "JK", "JC",
	"大小姐", "性转", "伪娘", "人外",
	"后宫", "百合", "耽美", "NTR", "女性视角",
}

type exploreResultMsg struct {
	subTab     ExploreSubTab
	tag        string
	results    []model.BookSummary
	totalPages int
	page       int
	err        error
}

// ExploreView displays online leaderboards and categorized tags.
type ExploreView struct {
	src        source.DataSource
	store      *storage.Storage
	subTab     ExploreSubTab
	tags       []string
	tagIndex   int
	results    []model.BookSummary
	cursor     int
	offset     int
	page       int
	totalPages int
	loading    bool
	err        error
	width      int
	height     int
	onSelect   func(bookID string) tea.Cmd
	onDownload func(bookID string) tea.Cmd
	onExport   func(bookID string, volumeIndex int) tea.Cmd
}

// NewExploreView creates a new explore and toplists view.
func NewExploreView(src source.DataSource, store *storage.Storage, onSelect func(bookID string) tea.Cmd) *ExploreView {
	tags := defaultExploreTags
	if src != nil {
		srcTags := src.GetTags()
		if len(srcTags) > 0 {
			tags = srcTags
		}
	}

	return &ExploreView{
		src:        src,
		store:      store,
		subTab:     SubTabHot,
		tags:       tags,
		tagIndex:   0,
		results:    make([]model.BookSummary, 0),
		page:       1,
		totalPages: 1,
		onSelect:   onSelect,
	}
}

// SetOnDownload registers download callback.
func (v *ExploreView) SetOnDownload(fn func(bookID string) tea.Cmd) {
	v.onDownload = fn
}

// SetOnExport registers export callback.
func (v *ExploreView) SetOnExport(fn func(bookID string, volumeIndex int) tea.Cmd) {
	v.onExport = fn
}

// SetResults populates explore results directly (useful for tests).
func (v *ExploreView) SetResults(results []model.BookSummary) {
	v.loading = false
	v.results = results
	v.err = nil
	v.cursor = 0
	v.offset = 0
	v.page = 1
	v.totalPages = 1
}

// Init triggers initial loading of the first leaderboard.
func (v *ExploreView) Init() tea.Cmd {
	return v.fetchCmd(v.subTab, 1)
}

// SetSize updates layout dimensions.
func (v *ExploreView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

func (v *ExploreView) fetchCmd(tab ExploreSubTab, page int) tea.Cmd {
	if v.src == nil {
		return nil
	}
	if page < 1 {
		page = 1
	}
	v.loading = true
	v.err = nil

	currentTag := ""
	if tab == SubTabTags && len(v.tags) > 0 {
		if v.tagIndex < 0 || v.tagIndex >= len(v.tags) {
			v.tagIndex = 0
		}
		currentTag = v.tags[v.tagIndex]
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		var res []model.BookSummary
		var total int
		var err error

		switch tab {
		case SubTabHot:
			res, total, err = v.src.GetToplist(ctx, source.ToplistHot, page)
		case SubTabAnime:
			res, total, err = v.src.GetToplist(ctx, source.ToplistAnime, page)
		case SubTabUpdate:
			res, total, err = v.src.GetToplist(ctx, source.ToplistLastUpdate, page)
		case SubTabPostDate:
			res, total, err = v.src.GetToplist(ctx, source.ToplistPostDate, page)
		case SubTabCompleted:
			res, total, err = v.src.GetToplist(ctx, source.ToplistCompleted, page)
		case SubTabTags:
			res, total, err = v.src.GetTagBooks(ctx, currentTag, page)
		}

		return exploreResultMsg{
			subTab:     tab,
			tag:        currentTag,
			results:    res,
			totalPages: total,
			page:       page,
			err:        err,
		}
	}
}

func (v *ExploreView) visibleCards() int {
	// Fixed lines:
	// Line 1: Header (1)
	// Line 2: Sub-tab badges (1)
	// Line 3: Category description & hint (1)
	// Line 4: Stats & Action hint (1)
	// Line 5: Top fold indicator (1)
	// Line N: Bottom fold indicator (1)
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

func (v *ExploreView) adjustOffset() {
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

// Update handles user interaction in ExploreView.
func (v *ExploreView) Update(msg tea.Msg) (*ExploreView, tea.Cmd) {
	switch msg := msg.(type) {
	case exploreResultMsg:
		v.loading = false
		v.results = msg.results
		v.totalPages = msg.totalPages
		v.page = msg.page
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
		case "left", "h":
			if v.subTab > SubTabHot {
				v.subTab--
				v.page = 1
				return v, v.fetchCmd(v.subTab, 1)
			}
		case "right", "l":
			if v.subTab < SubTabTags {
				v.subTab++
				v.page = 1
				return v, v.fetchCmd(v.subTab, 1)
			}
		case "t":
			if v.subTab == SubTabTags && len(v.tags) > 0 {
				v.tagIndex = (v.tagIndex + 1) % len(v.tags)
				v.page = 1
				return v, v.fetchCmd(v.subTab, 1)
			} else {
				// Quick cycle tabs
				v.subTab = (v.subTab + 1) % 6
				v.page = 1
				return v, v.fetchCmd(v.subTab, 1)
			}
		case "T":
			if v.subTab == SubTabTags && len(v.tags) > 0 {
				v.tagIndex = (v.tagIndex - 1 + len(v.tags)) % len(v.tags)
				v.page = 1
				return v, v.fetchCmd(v.subTab, 1)
			} else {
				v.subTab = (v.subTab - 1 + 6) % 6
				v.page = 1
				return v, v.fetchCmd(v.subTab, 1)
			}
		case "r", "R":
			return v, v.fetchCmd(v.subTab, v.page)
		case "[", "p":
			if v.page > 1 {
				v.page--
				return v, v.fetchCmd(v.subTab, v.page)
			}
		case "]", "n":
			if v.page < v.totalPages {
				v.page++
				return v, v.fetchCmd(v.subTab, v.page)
			}
		case "down", "j", "ctrl+j":
			if len(v.results) > 0 && v.cursor < len(v.results)-1 {
				v.cursor++
				v.adjustOffset()
			}
		case "up", "k", "ctrl+k":
			if len(v.results) > 0 && v.cursor > 0 {
				v.cursor--
				v.adjustOffset()
			}
		case "pgup", "ctrl+u", "b":
			if len(v.results) > 0 {
				v.cursor -= visible
				if v.cursor < 0 {
					v.cursor = 0
				}
				v.adjustOffset()
			}
		case "pgdown", "ctrl+d", "f":
			if len(v.results) > 0 {
				v.cursor += visible
				if v.cursor >= len(v.results) {
					v.cursor = len(v.results) - 1
				}
				v.adjustOffset()
			}
		case "g", "home":
			if len(v.results) > 0 {
				v.cursor = 0
				v.offset = 0
			}
		case "G", "end":
			if len(v.results) > 0 {
				v.cursor = len(v.results) - 1
				v.adjustOffset()
			}
		case "enter":
			if len(v.results) > 0 && v.cursor < len(v.results) {
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
		case "d", "c":
			if len(v.results) > 0 && v.cursor < len(v.results) {
				if v.onDownload != nil {
					return v, v.onDownload(v.results[v.cursor].ID)
				}
			}
		case "e":
			if len(v.results) > 0 && v.cursor < len(v.results) {
				if v.onExport != nil {
					return v, v.onExport(v.results[v.cursor].ID, common.ExportModeFullBook)
				}
			}
		case "s":
			if len(v.results) > 0 && v.cursor < len(v.results) {
				if v.onExport != nil {
					return v, v.onExport(v.results[v.cursor].ID, common.ExportModeAllVolumesSeparate)
				}
			}
		}
	}

	return v, nil
}

// View renders the ExploreView.
func (v *ExploreView) View() string {
	var sb strings.Builder

	header := lipgloss.NewStyle().Bold(true).Foreground(theme.PrimaryColor).
		Render("在线轻小说发现与排行榜 (Wenku8)")
	sb.WriteString(header + "\n")

	maxWidth := v.width - 2
	if maxWidth < 30 {
		maxWidth = 30
	}

	// Line 2: Sub-tabs
	subTabs := []struct {
		tab   ExploreSubTab
		label string
	}{
		{SubTabHot, "热门榜"},
		{SubTabAnime, "动画化"},
		{SubTabUpdate, "今日更新"},
		{SubTabPostDate, "新书榜"},
		{SubTabCompleted, "完结全本"},
		{SubTabTags, "题材分类"},
	}

	var badges []string
	for _, item := range subTabs {
		if item.tab == v.subTab {
			badges = append(badges, lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.TextWhite).
				Background(theme.PrimaryColor).
				Padding(0, 1).
				Render(fmt.Sprintf("[%s]", item.label)))
		} else {
			badges = append(badges, lipgloss.NewStyle().
				Foreground(theme.TextMuted).
				Render(fmt.Sprintf("[%s]", item.label)))
		}
	}
	topBar := " 榜单类型: " + strings.Join(badges, " ") + lipgloss.NewStyle().Foreground(theme.TextDim).Render("  (按 [←/→] 或 [h/l] 切换)")
	sb.WriteString(runewidth.Truncate(topBar, maxWidth, "...") + "\n")

	// Line 3: Description & category details
	var detailText string
	switch v.subTab {
	case SubTabHot:
		detailText = "当前榜单: 热门轻小说 (按总访问量排行)  •  按 [r] 刷新"
	case SubTabAnime:
		detailText = "当前榜单: 已动画化轻小说作品  •  按 [r] 刷新"
	case SubTabUpdate:
		detailText = "当前榜单: 今日最新更新小说章节  •  按 [r] 刷新"
	case SubTabPostDate:
		detailText = "当前榜单: 新书入库一览  •  按 [r] 刷新"
	case SubTabCompleted:
		detailText = "当前榜单: 完结全本精选轻小说  •  按 [r] 刷新"
	case SubTabTags:
		currentTag := "校园"
		if len(v.tags) > 0 && v.tagIndex < len(v.tags) {
			currentTag = v.tags[v.tagIndex]
		}
		detailText = fmt.Sprintf("当前题材: [%s] (第 %d/%d 个)  •  按 [t/T] 轮换分类题材  •  按 [r] 刷新",
			currentTag, v.tagIndex+1, len(v.tags))
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentSky).
		Render(runewidth.Truncate(" "+detailText, maxWidth, "...")) + "\n")

	// Loading state
	if v.loading {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentSky).
			Render(" [加载中] 正在联网获取榜单数据，请稍候...") + "\n")
		return sb.String()
	}

	// Error state
	if v.err != nil {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentRose).
			Render(fmt.Sprintf(" [错误] 获取榜单出错: %v", v.err)) + "\n")
		return sb.String()
	}

	// Empty state
	if len(v.results) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.TextDim).
			Render(" [提示] 暂无数据，请尝试按 [r] 重新获取，或按 [←/→] 切换其他榜单。") + "\n")
		return sb.String()
	}

	// Line 4: Stats & action hint
	curPos := v.cursor + 1
	pageStr := ""
	if v.totalPages > 1 {
		pageStr = fmt.Sprintf("第 %d/%d 页 ([/])  •  ", v.page, v.totalPages)
	}
	statsText := fmt.Sprintf(" %s共 %d 本  •  当前 [%d/%d]  •  [Enter] 目录  •  [d] 下载  •  [e] 导出",
		pageStr, len(v.results), curPos, len(v.results))
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.PrimaryLight).
		Render(runewidth.Truncate(statsText, maxWidth, "...")) + "\n")

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

	// Render Cards
	for i := start; i < end; i++ {
		b := v.results[i]
		isSelected := i == v.cursor

		var line1Badges string
		if b.IsComplete {
			line1Badges = theme.BadgeSuccess.Render("完结")
		}

		desc := strings.ReplaceAll(b.Description, "\r", " ")
		desc = strings.ReplaceAll(desc, "\n", " ")
		desc = strings.TrimSpace(desc)
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

		if isSelected {
			// Line 1: Title + Badges
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
			if runewidth.StringWidth(b.Title) > titleBudget {
				titleTrunc = runewidth.Truncate(b.Title, titleBudget, "...")
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

			// Line 2: Meta Info
			metaContent := fmt.Sprintf("%s作者: %s    文库: %s    字数: %s    状态: %s    ID: #%s",
				barActiveIndent, b.Author, pubStr, wordCountStr, statusStr, b.ID)
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
			if runewidth.StringWidth(b.Title) > titleBudget {
				titleTrunc = runewidth.Truncate(b.Title, titleBudget, "...")
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

			// Line 2: Meta Info
			metaContent := fmt.Sprintf("%s作者: %s    文库: %s    字数: %s    状态: %s    ID: #%s",
				barInactive, b.Author, pubStr, wordCountStr, statusStr, b.ID)
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

	// Line N: Bottom fold indicator (strictly 1 line)
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
		msg := fmt.Sprintf("  [全部] 已显示全部 %d 部作品 ", len(v.results))
		ruleLen := maxWidth - runewidth.StringWidth(msg)
		if ruleLen < 0 {
			ruleLen = 0
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(theme.AccentEmerald).
			Render(msg+strings.Repeat("─", ruleLen)) + "\n")
	}

	return sb.String()
}
