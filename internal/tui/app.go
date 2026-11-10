package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"telegram_feed_summarizer/internal/domain"
)

// Run launches the interactive Terminal User Interface.
func Run(repo domain.Repository) error {
	m := NewModel(repo)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
