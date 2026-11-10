package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"telegram_feed_summarizer/internal/domain"
	"telegram_feed_summarizer/internal/service"
)

// Run launches the interactive Terminal User Interface.
func Run(repo domain.Repository, feedSvc *service.FeedService) error {
	m := NewModel(repo, feedSvc)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
