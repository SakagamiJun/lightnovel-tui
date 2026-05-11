package theme

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Modern Vibrant & High-Contrast Colors (Linear / Tailwind Inspired)
	PrimaryColor   = lipgloss.Color("#6366F1") // Electric Indigo
	PrimaryLight   = lipgloss.Color("#818CF8") // Soft Indigo Accent
	SecondaryColor = lipgloss.Color("#4F46E5") // Deep Indigo
	AccentEmerald  = lipgloss.Color("#10B981") // Crisp Emerald Green
	AccentAmber    = lipgloss.Color("#F59E0B") // Warm Amber / Orange
	AccentSky      = lipgloss.Color("#0EA5E9") // Luminous Sky Blue
	AccentRose     = lipgloss.Color("#F43F5E") // Modern Rose Red
	TextWhite      = lipgloss.Color("#F8FAFC") // Crisp White
	TextMuted      = lipgloss.Color("#94A3B8") // Slate Gray
	TextDim        = lipgloss.Color("#64748B") // Dim Cool Gray
	BorderColor    = lipgloss.Color("#334155") // Subtle Slate Border
	HighlightBg    = lipgloss.Color("#1E1B4B") // Deep Indigo Glow Background
	SurfaceBg      = lipgloss.Color("#0F172A") // Deep Midnight Surface
	BarBg          = lipgloss.Color("#1E293B") // Slate Bar Background

	// Compatibility aliases
	AccentColor  = AccentEmerald
	MutedColor   = TextMuted
	WarningColor = AccentRose

	// App Header & Tabs
	AppTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(TextWhite).
			Background(PrimaryColor).
			Padding(0, 1).
			MarginRight(1)

	TabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(TextWhite).
			Background(SecondaryColor).
			Padding(0, 2)

	TabInactiveStyle = lipgloss.NewStyle().
				Foreground(TextMuted).
				Background(BarBg).
				Padding(0, 2)

	HeaderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(BorderColor).
			Padding(0, 1)

	// Status Bar
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(TextMuted).
			Background(BarBg).
			Padding(0, 1)

	StatusKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryLight)

	// Badge Styles
	BadgeSuccess = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#34D399")).
			Background(lipgloss.Color("#064E3B")).
			Padding(0, 1)

	BadgeWarning = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FBBF24")).
			Background(lipgloss.Color("#78350F")).
			Padding(0, 1)

	BadgeInfo = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#38BDF8")).
			Background(lipgloss.Color("#0C4A6E")).
			Padding(0, 1)

	BadgeMuted = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CBD5E1")).
			Background(lipgloss.Color("#334155")).
			Padding(0, 1)

	// Search & Card Styles
	CardTitleActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(TextWhite)

	CardTitleInactive = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#E2E8F0"))

	CardMetaActive = lipgloss.NewStyle().
			Foreground(PrimaryLight)

	CardMetaInactive = lipgloss.NewStyle().
				Foreground(TextMuted)

	CardDescActive = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A5B4FC"))

	CardDescInactive = lipgloss.NewStyle().
				Foreground(TextDim)

	CardBorderActive = lipgloss.NewStyle().
				Bold(true).
				Foreground(PrimaryLight)

	CardBorderInactive = lipgloss.NewStyle().
				Foreground(BorderColor)

	ListItemStyle = lipgloss.NewStyle().
			Padding(0, 1)

	ListItemActiveStyle = lipgloss.NewStyle().
				Background(HighlightBg).
				Padding(0, 1)

	TagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)
)

// FormatWordCount formats word count into human-friendly Chinese notation (万字).
func FormatWordCount(words int) string {
	if words >= 10000 {
		return fmt.Sprintf("%.1f万字", float64(words)/10000.0)
	}
	if words > 0 {
		return fmt.Sprintf("%d字", words)
	}
	return "未知字数"
}
