package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/theme"
)

var (
	colorHeat0 = lipgloss.Color("#475569") // 0 min (dim slate)
	colorHeat1 = lipgloss.Color("#065F46") // 1-15 min (dark emerald)
	colorHeat2 = lipgloss.Color("#059669") // 15-45 min (medium emerald)
	colorHeat3 = lipgloss.Color("#10B981") // 45-90 min (emerald)
	colorHeat4 = lipgloss.Color("#34D399") // 90+ min (bright emerald)

	dayLabels = []string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}
)

// formatWordCount formats word count into a human-readable Chinese string.
func formatWordCount(words int) string {
	if words >= 10000 {
		return fmt.Sprintf("%.1f 万字", float64(words)/10000.0)
	}
	return fmt.Sprintf("%d 字", words)
}

// RenderStatsModal renders the personal reading stats & heatmap modal.
func RenderStatsModal(store *storage.Storage, width, height int) string {
	modalW := width - 6
	if modalW > 68 {
		modalW = 68
	}
	if modalW < 36 {
		modalW = 36
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.PrimaryColor)

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.AccentSky)

	labelStyle := lipgloss.NewStyle().
		Foreground(theme.TextWhite)

	valueStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.AccentAmber)

	mutedStyle := lipgloss.NewStyle().
		Foreground(theme.MutedColor)

	var sb strings.Builder

	// Header
	header := titleStyle.Render("[ 个人阅读统计与打卡热力图 ]")
	sb.WriteString(header + "\n\n")

	if store == nil {
		sb.WriteString(mutedStyle.Render("暂无统计数据\n"))
		return renderModalBox(sb.String(), modalW)
	}

	// 1. Heatmap (past 12 weeks)
	sb.WriteString(sectionStyle.Render("最近 12 周打卡记录:") + "\n")
	weeks := store.GetHeatmapWeeks(12)

	for d := 0; d < 7; d++ {
		sb.WriteString(mutedStyle.Render(dayLabels[d]) + "  ")
		for w := 0; w < len(weeks); w++ {
			mins := weeks[w].Days[d]
			var cellStyle lipgloss.Style
			char := "■"
			switch {
			case mins == 0:
				cellStyle = lipgloss.NewStyle().Foreground(colorHeat0)
				char = "·"
			case mins < 15:
				cellStyle = lipgloss.NewStyle().Foreground(colorHeat1)
			case mins < 45:
				cellStyle = lipgloss.NewStyle().Foreground(colorHeat2)
			case mins < 90:
				cellStyle = lipgloss.NewStyle().Foreground(colorHeat3)
			default:
				cellStyle = lipgloss.NewStyle().Foreground(colorHeat4)
			}
			sb.WriteString(cellStyle.Render(char) + " ")
		}
		sb.WriteString("\n")
	}

	// Heatmap Legend
	sb.WriteString("\n" + mutedStyle.Render("图例: ") +
		lipgloss.NewStyle().Foreground(colorHeat0).Render("·") + mutedStyle.Render(" 0分  ") +
		lipgloss.NewStyle().Foreground(colorHeat1).Render("■") + mutedStyle.Render(" 1-15分  ") +
		lipgloss.NewStyle().Foreground(colorHeat2).Render("■") + mutedStyle.Render(" 15-45分  ") +
		lipgloss.NewStyle().Foreground(colorHeat3).Render("■") + mutedStyle.Render(" 45-90分  ") +
		lipgloss.NewStyle().Foreground(colorHeat4).Render("■") + mutedStyle.Render(" 90分+\n\n"))

	// 2. Aggregate Metrics
	stats, err := store.LoadStats()
	if err != nil || stats == nil {
		stats = &storage.OverallStats{}
	}

	todayStr := time.Now().Format("2006-01-02")
	todayStat := stats.DailyRecords[todayStr]
	todayMins := todayStat.ReadSeconds / 60
	todayWords := formatWordCount(todayStat.EstimatedWords)

	totalHours := stats.TotalSeconds / 3600
	totalMins := (stats.TotalSeconds % 3600) / 60
	totalWords := formatWordCount(stats.TotalWords)

	sb.WriteString(sectionStyle.Render("[核心指标]") + "\n")
	sb.WriteString(fmt.Sprintf("%s %s %s\n",
		labelStyle.Render("• 今日阅读:"),
		valueStyle.Render(fmt.Sprintf("%d 分钟", todayMins)),
		mutedStyle.Render(fmt.Sprintf("(约 %s)", todayWords))))

	sb.WriteString(fmt.Sprintf("%s %s %s\n",
		labelStyle.Render("• 连续打卡:"),
		valueStyle.Render(fmt.Sprintf("%d 天", stats.CurrentStreakDays)),
		mutedStyle.Render(fmt.Sprintf("(历史最高: %d 天)", stats.MaxStreakDays))))

	sb.WriteString(fmt.Sprintf("%s %s %s\n",
		labelStyle.Render("• 累计阅读:"),
		valueStyle.Render(fmt.Sprintf("%d 小时 %d 分钟", totalHours, totalMins)),
		mutedStyle.Render(fmt.Sprintf("(累计约 %s)", totalWords))))

	recent := store.GetRecentBook()
	if recent != nil && recent.Title != "" {
		chapInfo := recent.LastReadChapter
		if chapInfo == "" {
			chapInfo = "正文"
		}
		sb.WriteString(fmt.Sprintf("%s %s %s\n",
			labelStyle.Render("• 最近在读:"),
			valueStyle.Render(fmt.Sprintf("《%s》%s", recent.Title, chapInfo)),
			lipgloss.NewStyle().Foreground(theme.AccentEmerald).Render(fmt.Sprintf("(进度 %.1f%%)", recent.ProgressPercent))))
	} else {
		sb.WriteString(labelStyle.Render("• 最近在读: ") + mutedStyle.Render("暂无在读记录\n"))
	}

	// 3. Footer Shortcuts
	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(theme.BorderColor).Render(strings.Repeat("─", modalW-4)) + "\n")
	footer := lipgloss.NewStyle().Foreground(theme.MutedColor).Render("[Enter/c] 极速续读最近小说  │  [Esc/s] 关闭浮层")
	sb.WriteString(footer)

	return renderModalBox(sb.String(), modalW)
}

func renderModalBox(content string, width int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.PrimaryColor).
		Padding(1, 2).
		Width(width).
		Render(content)
}
