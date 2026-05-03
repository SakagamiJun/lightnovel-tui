package theme

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	PrimaryColor   = lipgloss.Color("#7D56F4")
	SecondaryColor = lipgloss.Color("#5A3EBC")
	AccentColor    = lipgloss.Color("#04B575")
	MutedColor     = lipgloss.Color("#626262")
	WarningColor   = lipgloss.Color("#FF5F87")
	BorderColor    = lipgloss.Color("#383838")
	HighlightBg    = lipgloss.Color("#2A2438")

	// Styles
	AppTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(PrimaryColor).
			Padding(0, 1).
			MarginRight(1)

	TabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(SecondaryColor).
			Padding(0, 2)

	TabInactiveStyle = lipgloss.NewStyle().
				Foreground(MutedColor).
				Padding(0, 2)

	HeaderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(BorderColor).
			Padding(0, 1)

	StatusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D9D9D9")).
			Background(lipgloss.Color("#1E1E1E")).
			Padding(0, 1)

	StatusKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor)

	ListItemStyle = lipgloss.NewStyle().
			Padding(0, 1)

	ListItemActiveStyle = lipgloss.NewStyle().
				Background(HighlightBg).
				Padding(0, 1)

	CardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(BorderColor).
			Padding(0, 1)

	CardActiveStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(PrimaryColor).
			Background(HighlightBg).
			Padding(0, 1)

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E0E0E0"))

	AuthorStyle = lipgloss.NewStyle().
			Foreground(AccentColor)

	TagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)
)
