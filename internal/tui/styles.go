package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Theme colors
	primaryColor   = lipgloss.Color("#7D56F4")
	secondaryColor = lipgloss.Color("#5A3BC7")
	accentColor    = lipgloss.Color("#04B575")
	dangerColor    = lipgloss.Color("#FF4F60")
	subtleColor    = lipgloss.Color("#626262")
	highlightColor = lipgloss.Color("#FFD700")

	// Base styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primaryColor).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E0E0E0")).
			Background(secondaryColor)

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#3A2F60"))

	// Fixed width columns for channels table (prevents ANSI padding drift)
	colPrefixStyle = lipgloss.NewStyle().Width(3)
	colSyncStyle   = lipgloss.NewStyle().Width(8)
	colLLMStyle    = lipgloss.NewStyle().Width(7)
	colAdStyle     = lipgloss.NewStyle().Width(9)
	colTitleStyle  = lipgloss.NewStyle().Width(34)
	colTopicsStyle = lipgloss.NewStyle().Width(24)

	activeTagStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)

	inactiveTagStyle = lipgloss.NewStyle().
				Foreground(subtleColor)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A0A0A0")).
			Background(lipgloss.Color("#1A1A24")).
			Padding(0, 1)

	helpKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlightColor)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C0C0C0"))
)
