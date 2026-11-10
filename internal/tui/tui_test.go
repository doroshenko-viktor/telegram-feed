package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"telegram_feed_summarizer/internal/config"
	"telegram_feed_summarizer/internal/domain"
	"telegram_feed_summarizer/internal/service"
	"telegram_feed_summarizer/internal/storage"
)

func setupTestTUI(t *testing.T) (*Model, domain.Repository, *service.FeedService) {
	ctx := context.Background()
	cfg := config.StorageConfig{Driver: "sqlite", DSN: ":memory:"}
	repo, err := storage.NewRepository(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	feedSvc := service.NewFeedService(&config.Config{}, repo, nil, nil)
	m := NewModel(repo, feedSvc)
	return m, repo, feedSvc
}

func TestTUI_ModelInitAndChannelsView(t *testing.T) {
	m, repo, _ := setupTestTUI(t)
	ctx := context.Background()

	_ = repo.UpsertChannels(ctx, []*domain.Channel{
		{
			ID:         1,
			AccessHash: 123,
			Title:      "Golang Weekly",
			Username:   "golang_weekly",
			IsTracked:  true,
			MCPAllowed: true,
			FilterAds:  true,
			Topics:     []string{"go"},
		},
		{
			ID:         2,
			AccessHash: 456,
			Title:      "Kubernetes Insights",
			Username:   "k8s_insights",
			IsTracked:  false,
			MCPAllowed: false,
			FilterAds:  false,
			Topics:     []string{"k8s"},
		},
	})
	m.reloadChannels()

	if len(m.channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(m.channels))
	}

	viewStr := m.View()
	if !strings.Contains(viewStr, "Golang Weekly") {
		t.Errorf("expected view to contain 'Golang Weekly', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Local Storage") {
		t.Errorf("expected view to contain 'Local Storage', got:\n%s", viewStr)
	}
}

func TestTUI_DigestTriggerKey(t *testing.T) {
	m, repo, _ := setupTestTUI(t)
	ctx := context.Background()

	_ = repo.UpsertChannels(ctx, []*domain.Channel{
		{
			ID:        1,
			Title:     "DevOps Channel",
			IsTracked: true,
			Topics:    []string{"devops"},
		},
	})
	now := time.Now()
	_ = repo.UpsertPosts(ctx, []*domain.Post{
		{
			ChannelID: 1,
			MsgID:     1,
			Date:      now,
			Text:      "New Prometheus release v3.0 is out!",
			Views:     1000,
			Forwards:  10,
		},
	})
	m.reloadChannels()

	// Press [g] to open digest menu
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	newModel, _ := m.Update(keyMsg)
	m = newModel.(*Model)

	if !m.showingDigestMenu {
		t.Fatal("expected showingDigestMenu to be true after pressing [g]")
	}

	// Press [enter] to confirm digest creation
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, cmd := m.Update(enterMsg)
	m = newModel.(*Model)

	if !m.isBusy {
		t.Error("expected model to be marked as busy after confirming digest menu")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for digest generation")
	}

	// Execute cmd
	resMsg := cmd()
	newModel, _ = m.Update(resMsg)
	m = newModel.(*Model)

	if m.isBusy {
		t.Error("expected model to not be busy after digest generation finished")
	}
	if m.activeView != ViewDigests {
		t.Errorf("expected active view to switch to ViewDigests, got %d", m.activeView)
	}
	if len(m.digests) != 1 {
		t.Fatalf("expected 1 digest loaded, got %d", len(m.digests))
	}
	if !strings.Contains(m.View(), "Prometheus release") {
		t.Errorf("expected digests view to preview Prometheus summary, got:\n%s", m.View())
	}
}

func TestTUI_ChannelDigestAndFullHeight(t *testing.T) {
	m, repo, _ := setupTestTUI(t)
	ctx := context.Background()

	_ = repo.UpsertChannels(ctx, []*domain.Channel{
		{
			ID:        42,
			Title:     "Target Channel",
			Username:  "target_channel",
			IsTracked: true,
		},
	})
	m.reloadChannels()

	// Verify 100% height padding
	m.height = 40
	m.width = 100
	rendered := m.View()
	lines := strings.Split(rendered, "\n")
	if len(lines) < 38 {
		t.Errorf("expected at least 38 lines for height 40, got %d", len(lines))
	}

	// Press [c] to open digest menu pre-selected for channel
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = newModel.(*Model)
	if !m.showingDigestMenu {
		t.Fatal("expected showingDigestMenu to be true after pressing [c]")
	}
	if m.digestMenuScope != 1 {
		t.Errorf("expected digestMenuScope to be 1 (Individual Channel), got %d", m.digestMenuScope)
	}

	modalView := m.View()
	if !strings.Contains(modalView, "Individual Channel: Target Channel") {
		t.Errorf("expected modal to display channel name, got:\n%s", modalView)
	}

	// Navigate period down with [j]
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newModel.(*Model)
	if m.digestMenuPeriod != 1 {
		t.Errorf("expected period 1 (Today), got %d", m.digestMenuPeriod)
	}

	// Cancel with [esc]
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = newModel.(*Model)
	if m.showingDigestMenu {
		t.Error("expected showingDigestMenu to be false after escape")
	}
}

func TestTUI_FullDigestReader(t *testing.T) {
	m, repo, _ := setupTestTUI(t)
	ctx := context.Background()

	now := time.Now()
	longSummary := "# Full Digest Summary\n\n## Section 1: Major Highlights\n- Point A\n- Point B\n\n## Section 2: Detailed Updates\nLine 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10\nLine 11\nLine 12\nLine 13\nLine 14\nLine 15\n"

	_ = repo.SaveDigest(ctx, &domain.Digest{
		ID:              "test-digest-1",
		Title:           "Weekly Tech Digest",
		Topic:           "tech",
		PeriodStart:     now.Add(-7 * 24 * time.Hour),
		PeriodEnd:       now,
		SummaryMarkdown: longSummary,
		PostCount:       25,
		CreatedAt:       now,
	})
	m.reloadDigests()

	// Switch to ViewDigests with limited height to test scrolling
	m.activeView = ViewDigests
	m.digestCursor = 0
	m.height = 15

	// Press [Enter] to enter reader mode
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*Model)

	if !m.readingFullDigest {
		t.Fatal("expected readingFullDigest to be true after pressing [Enter]")
	}

	readerView := m.View()
	if !strings.Contains(readerView, "Full Digest Summary") {
		t.Errorf("expected reader view to contain title 'Full Digest Summary', got:\n%s", readerView)
	}

	// Scroll down with [j]
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newModel.(*Model)
	if m.digestScrollOffset != 1 {
		t.Errorf("expected digestScrollOffset 1, got %d", m.digestScrollOffset)
	}

	// Exit reader mode with [q]
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = newModel.(*Model)
	if m.readingFullDigest {
		t.Error("expected readingFullDigest to be false after pressing [q]")
	}
	if m.activeView != ViewDigests {
		t.Errorf("expected still in ViewDigests, got %d", m.activeView)
	}
}

func TestTUI_PromptEditorInSettings(t *testing.T) {
	m, repo, _ := setupTestTUI(t)
	ctx := context.Background()

	// Switch to settings
	m.activeView = ViewSettings
	items := m.getSettingItems()

	// Locate prompt setting index
	promptIdx := -1
	for idx, item := range items {
		if item.Key == domain.SettingKeyDigestPrompt {
			promptIdx = idx
			break
		}
	}
	if promptIdx == -1 {
		t.Fatal("expected SettingKeyDigestPrompt to be present in settings items")
	}

	m.settingsCursor = promptIdx

	// Press [Enter] to open prompt editor
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*Model)

	if !m.editingPrompt {
		t.Fatal("expected editingPrompt to be true after pressing Enter on prompt setting")
	}

	viewStr := m.View()
	if !strings.Contains(viewStr, "EDIT AI DIGEST PROMPT TEMPLATE") {
		t.Errorf("expected prompt editor view to contain title, got:\n%s", viewStr)
	}

	// Change prompt text
	customPrompt := "Custom prompt: focus only on high-performance Golang and Kubernetes tricks for [{{topic}}]."
	m.promptTextArea.SetValue(customPrompt)

	// Press [ctrl+s] to save
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = newModel.(*Model)

	if m.editingPrompt {
		t.Fatal("expected editingPrompt to be false after ctrl+s")
	}

	// Verify saved to database
	savedPrompt, err := repo.GetSetting(ctx, domain.SettingKeyDigestPrompt, "")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if savedPrompt != customPrompt {
		t.Errorf("expected saved prompt %q, got %q", customPrompt, savedPrompt)
	}

	// Re-open editor, test [ctrl+r] to reset
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*Model)
	if !m.editingPrompt {
		t.Fatal("expected editingPrompt true on re-open")
	}

	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = newModel.(*Model)
	if m.promptTextArea.Value() != domain.DefaultDigestPrompt {
		t.Errorf("expected prompt textarea to be reset to DefaultDigestPrompt after ctrl+r")
	}

	// Press [Esc] to cancel without saving
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newModel.(*Model)
	if m.editingPrompt {
		t.Fatal("expected editingPrompt to be false after esc")
	}
}

func TestTUI_ChannelDetailsAndTagEditor(t *testing.T) {
	m, repo, _ := setupTestTUI(t)
	ctx := context.Background()

	_ = repo.UpsertChannels(ctx, []*domain.Channel{
		{
			ID:         901,
			Title:      "Go Engineering",
			Username:   "go_eng",
			About:      "Deep dives into Go compiler internals and runtime latency.",
			Summary:    "Compiler optimizations and memory allocators.",
			Topics:     []string{"go", "backend"},
			IsTracked:  true,
			MCPAllowed: true,
			FilterAds:  true,
		},
	})
	m.reloadChannels()

	// 1. Press [Enter] on channel to open Channel Details
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*Model)

	if !m.showingChannelDetails {
		t.Fatal("expected showingChannelDetails to be true after Enter")
	}

	detailsView := m.View()
	if !strings.Contains(detailsView, "CHANNEL DETAILS: Go Engineering") {
		t.Errorf("expected view to contain channel title, got:\n%s", detailsView)
	}
	if !strings.Contains(detailsView, "#go") || !strings.Contains(detailsView, "#backend") {
		t.Errorf("expected view to contain tag badges, got:\n%s", detailsView)
	}

	// 2. Press [t] to edit tags
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = newModel.(*Model)

	if !m.editingChannelTags {
		t.Fatal("expected editingChannelTags to be true after [t]")
	}

	// 3. Update tags to: "go, kubernetes, cloud, highload"
	m.channelTagsInput.SetValue("go, kubernetes, cloud, highload")

	// Press [Enter] to save tags
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*Model)

	if m.editingChannelTags {
		t.Fatal("expected editingChannelTags false after saving")
	}

	// Verify tags in database
	ch, err := repo.GetChannel(ctx, 901)
	if err != nil {
		t.Fatalf("GetChannel failed: %v", err)
	}
	if len(ch.Topics) != 4 || ch.Topics[1] != "kubernetes" {
		t.Errorf("expected 4 updated topics in DB, got: %#v", ch.Topics)
	}

	// 4. Press [Esc] to return to channels list
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newModel.(*Model)

	if m.showingChannelDetails {
		t.Fatal("expected showingChannelDetails to be false after Esc")
	}
}

func TestTUI_DigestByTagsScope(t *testing.T) {
	m, repo, _ := setupTestTUI(t)
	ctx := context.Background()

	_ = repo.UpsertChannels(ctx, []*domain.Channel{
		{
			ID:        950,
			Title:     "K8s News",
			Topics:    []string{"kubernetes", "devops"},
			IsTracked: true,
		},
	})
	m.reloadChannels()

	// Press [g] to open digest menu
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = newModel.(*Model)

	if !m.showingDigestMenu || m.digestMenuScope != 0 {
		t.Fatalf("expected digest menu with scope 0, got showing: %v, scope: %d", m.showingDigestMenu, m.digestMenuScope)
	}

	// Press [Tab] twice to cycle to Scope 2 (By Topic / Tag(s))
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = newModel.(*Model)
	if m.digestMenuScope != 1 {
		t.Fatalf("expected scope 1 after first tab, got %d", m.digestMenuScope)
	}

	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = newModel.(*Model)
	if m.digestMenuScope != 2 {
		t.Fatalf("expected scope 2 (By Topic) after second tab, got %d", m.digestMenuScope)
	}

	viewStr := m.View()
	if !strings.Contains(viewStr, "By Topic / Tag(s)") {
		t.Errorf("expected view to contain 'By Topic / Tag(s)', got:\n%s", viewStr)
	}

	// Set topic input
	m.digestTopicInput.SetValue("kubernetes + devops")

	// Press [Enter] to generate
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*Model)

	if m.showingDigestMenu {
		t.Fatal("expected digest menu to close after Enter")
	}
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd for digest generation")
	}
}
