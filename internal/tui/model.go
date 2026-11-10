package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"telegram_feed_summarizer/internal/domain"
	"telegram_feed_summarizer/internal/llm/gemini"
	"telegram_feed_summarizer/internal/service"
)

type ViewMode int

const (
	ViewChannels ViewMode = iota
	ViewSettings
	ViewPatterns
	ViewDigests
)

// SettingItem defines a configurable application setting in the TUI.
type SettingItem struct {
	Key         string
	Label       string
	Description string
	IsToggle    bool
	IsCycle     bool
	CycleOpts   []string
	Value       string
}

// Model represents the Bubble Tea TUI state.
type Model struct {
	repo             domain.Repository
	feedSvc          *service.FeedService
	channels         []*domain.Channel
	filteredChannels []*domain.Channel
	cursor           int
	activeView       ViewMode

	// Filter / Search
	filtering   bool
	filterInput textinput.Model

	// Application Settings
	settingsCursor     int
	editingSetting     bool
	settingInput       textinput.Model
	telegramAppID      string
	hasAppHash         bool
	geminiKey          string
	geminiModel        string
	botToken           string
	botChatID          string
	deliveryBotEnabled string
	cronSchedule       string
	filterAdsGlobal    string
	minPostLength      string
	digestPrompt       string
	userInterests      string
	userIgnoredTopics  string
	editingPrompt      bool
	promptTextArea     textarea.Model

	// Dynamic Ad Patterns
	adPatterns    []*domain.AdPattern
	patternCursor int
	newPatternIn  textinput.Model
	addingPattern bool

	// Digests
	digests      []*domain.Digest
	digestCursor int

	// Local Message Cache Stats
	totalPosts  int
	unreadPosts int

	// Background Task Execution
	isBusy     bool
	busyAction string

	// Full-Height Scrolling
	scrollOffset int

	// Channel Details & Tag Editor
	showingChannelDetails bool
	editingChannelTags    bool
	channelTagsInput      textinput.Model

	// Digest Creation Menu Dialog
	showingDigestMenu bool
	digestMenuScope   int // 0 = All Tracked Channels, 1 = Selected Channel, 2 = By Topic / Tags
	digestMenuPeriod  int // 0 = Unread, 1 = Today, 2 = Past 7 Days, 3 = Past 30 Days, 4 = Current Month, 5 = Previous Month
	digestTopicInput  textinput.Model

	// Full Digest Reader Mode
	readingFullDigest  bool
	digestScrollOffset int

	// Status
	statusMsg  string
	statusTime time.Time
	width      int
	height     int
}

// NewModel creates an initialized TUI model.
func NewModel(repo domain.Repository, feedSvc *service.FeedService) *Model {
	ti := textinput.New()
	ti.Placeholder = "type to filter channels by title, username or topic..."
	ti.Prompt = "/ "

	np := textinput.New()
	np.Placeholder = "new keyword or regex (e.g. 'скидка' or 't\\.me/\\+.*')..."
	np.Prompt = "+ "

	si := textinput.New()
	si.Prompt = "Enter value: "

	ta := textarea.New()
	ta.Placeholder = "Enter custom system prompt for AI digest generation..."
	ta.ShowLineNumbers = true
	ta.CharLimit = 0

	tiTags := textinput.New()
	tiTags.Placeholder = "e.g. go, programming, concurrency, ai..."
	tiTags.Prompt = "Tags: "

	tiTopic := textinput.New()
	tiTopic.Placeholder = "e.g. go + kubernetes, programming, ai..."
	tiTopic.Prompt = "Tags: "

	m := &Model{
		repo:             repo,
		feedSvc:          feedSvc,
		activeView:       ViewChannels,
		filterInput:      ti,
		newPatternIn:     np,
		settingInput:     si,
		promptTextArea:   ta,
		channelTagsInput: tiTags,
		digestTopicInput: tiTopic,
		width:            100,
		height:           30,
	}

	m.reloadChannels()
	m.reloadSettingsAndPatterns()
	m.reloadDigests()
	return m
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) reloadChannels() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	channels, err := m.repo.ListChannels(ctx, false)
	if err == nil {
		m.channels = channels
		m.applyFilter()
	}

	total, unread, err := m.repo.GetPostStats(ctx)
	if err == nil {
		m.totalPosts = total
		m.unreadPosts = unread
	}
}

func (m *Model) reloadSettingsAndPatterns() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	patterns, _ := m.repo.ListAdPatterns(ctx, false)
	m.adPatterns = patterns

	m.telegramAppID, _ = m.repo.GetSetting(ctx, domain.SettingKeyTelegramAppID, "")
	appHash, _ := m.repo.GetSetting(ctx, domain.SettingKeyTelegramAppHash, "")
	m.hasAppHash = (appHash != "")

	m.geminiKey, _ = m.repo.GetSetting(ctx, domain.SettingKeyGeminiAPIKey, "")
	m.geminiModel, _ = m.repo.GetSetting(ctx, domain.SettingKeyGeminiModel, "gemini-3.8-flash")

	m.botToken, _ = m.repo.GetSetting(ctx, domain.SettingKeyBotToken, "")
	m.botChatID, _ = m.repo.GetSetting(ctx, domain.SettingKeyBotChatID, "")
	m.deliveryBotEnabled, _ = m.repo.GetSetting(ctx, domain.SettingKeyDeliveryBot, "false")
	m.cronSchedule, _ = m.repo.GetSetting(ctx, domain.SettingKeyCronSchedule, "0 9 * * *")
	m.filterAdsGlobal, _ = m.repo.GetSetting(ctx, domain.SettingKeyFilterAds, "true")
	m.minPostLength, _ = m.repo.GetSetting(ctx, domain.SettingKeyMinPostLength, "20")
	m.digestPrompt, _ = m.repo.GetSetting(ctx, domain.SettingKeyDigestPrompt, domain.DefaultDigestPrompt)
	m.userInterests, _ = m.repo.GetSetting(ctx, domain.SettingKeyUserInterests, domain.DefaultUserInterests)
	m.userIgnoredTopics, _ = m.repo.GetSetting(ctx, domain.SettingKeyUserIgnoredTopics, domain.DefaultUserIgnoredTopics)
}

func (m *Model) getSettingItems() []SettingItem {
	geminiDisplay := "Not set (Pure MCP mode)"
	if m.geminiKey != "" {
		geminiDisplay = "••••••••••••••••"
	}
	botTokenDisplay := "Not set"
	if m.botToken != "" {
		if len(m.botToken) > 10 {
			botTokenDisplay = m.botToken[:6] + "..." + m.botToken[len(m.botToken)-4:]
		} else {
			botTokenDisplay = m.botToken
		}
	}
	botChatDisplay := m.botChatID
	if botChatDisplay == "" {
		botChatDisplay = "Not set"
	}

	promptDisplay := "Default template"
	if strings.TrimSpace(m.digestPrompt) != "" && strings.TrimSpace(m.digestPrompt) != strings.TrimSpace(domain.DefaultDigestPrompt) {
		lines := strings.Count(m.digestPrompt, "\n") + 1
		promptDisplay = fmt.Sprintf("Customized (%d lines)", lines)
	}

	return []SettingItem{
		{
			Key:         domain.SettingKeyDeliveryBot,
			Label:       "Telegram Bot Push Delivery",
			Description: "Deliver automated digests to private bot chat (Space to toggle)",
			IsToggle:    true,
			Value:       m.deliveryBotEnabled,
		},
		{
			Key:         domain.SettingKeyBotToken,
			Label:       "Telegram Bot Token",
			Description: "API token from @BotFather (Enter to edit)",
			Value:       botTokenDisplay,
		},
		{
			Key:         domain.SettingKeyBotChatID,
			Label:       "Telegram Bot Chat ID",
			Description: "Numeric user ID from @userinfobot (Enter to edit)",
			Value:       botChatDisplay,
		},
		{
			Key:         domain.SettingKeyCronSchedule,
			Label:       "Daemon Cron Schedule",
			Description: "Cron expression (e.g. 0 9 * * *) (Enter to edit)",
			Value:       m.cronSchedule,
		},
		{
			Key:         domain.SettingKeyGeminiAPIKey,
			Label:       "Google Gemini API Key",
			Description: "Only for local CLI summaries. Enter 'no' to clear (Enter to edit)",
			Value:       geminiDisplay,
		},
		{
			Key:         domain.SettingKeyGeminiModel,
			Label:       "Gemini Synthesis Model",
			Description: "Active AI model (Space to cycle)",
			IsCycle:     true,
			CycleOpts:   []string{"gemini-3.8-flash", "gemini-3.8-pro", "gemini-3.0-flash"},
			Value:       m.geminiModel,
		},
		{
			Key:         domain.SettingKeyDigestPrompt,
			Label:       "AI Digest Prompt Template",
			Description: "System prompt for AI summarization (Enter to edit full prompt)",
			Value:       promptDisplay,
		},
		{
			Key:         domain.SettingKeyUserInterests,
			Label:       "User High-Priority Interests",
			Description: "Topics you care about: tech breakthroughs, tools, AI (Enter to edit)",
			Value:       truncateRunes(m.userInterests, 24),
		},
		{
			Key:         domain.SettingKeyUserIgnoredTopics,
			Label:       "Noise & Ignored Topics",
			Description: "Topics to drop: gossip, scandals, game preloads, memes (Enter to edit)",
			Value:       truncateRunes(m.userIgnoredTopics, 24),
		},
		{
			Key:         domain.SettingKeyFilterAds,
			Label:       "Global Noise & Ad Filter",
			Description: "Drop promo messages matching ad patterns (Space to toggle)",
			IsToggle:    true,
			Value:       m.filterAdsGlobal,
		},
		{
			Key:         domain.SettingKeyMinPostLength,
			Label:       "Minimum Post Length",
			Description: "Drop noise messages shorter than N chars (Enter to edit)",
			Value:       m.minPostLength,
		},
	}
}

func (m *Model) reloadDigests() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	digests, _ := m.repo.ListDigests(ctx, "all", false, 20)
	m.digests = digests
}

func (m *Model) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))
	if query == "" {
		m.filteredChannels = m.channels
		if m.cursor >= len(m.filteredChannels) {
			m.cursor = max(0, len(m.filteredChannels)-1)
		}
		return
	}

	var matched []*domain.Channel
	for _, ch := range m.channels {
		if strings.Contains(strings.ToLower(ch.Title), query) ||
			strings.Contains(strings.ToLower(ch.Username), query) ||
			strings.Contains(strings.ToLower(strings.Join(ch.Topics, " ")), query) {
			matched = append(matched, ch)
		}
	}
	m.filteredChannels = matched
	if m.cursor >= len(m.filteredChannels) {
		m.cursor = max(0, len(m.filteredChannels)-1)
	}
}

func (m *Model) setStatus(msg string) {
	m.statusMsg = msg
	m.statusTime = time.Now()
}

type syncFinishedMsg struct {
	count int
	err   error
}

type digestFinishedMsg struct {
	digest *domain.Digest
	err    error
}

func syncPostsCmd(feedSvc *service.FeedService) tea.Cmd {
	return func() tea.Msg {
		if feedSvc == nil {
			return syncFinishedMsg{err: fmt.Errorf("feed service is not configured")}
		}
		count, err := feedSvc.SyncTrackedPosts(context.Background())
		return syncFinishedMsg{count: count, err: err}
	}
}

func generateDigestCmd(feedSvc *service.FeedService, topic string) tea.Cmd {
	return func() tea.Msg {
		if feedSvc == nil {
			return digestFinishedMsg{err: fmt.Errorf("feed service is not configured")}
		}
		digest, err := feedSvc.GenerateDigest(context.Background(), topic)
		return digestFinishedMsg{digest: digest, err: err}
	}
}

func generateDigestWithOptionsCmd(feedSvc *service.FeedService, opts service.DigestOptions) tea.Cmd {
	return func() tea.Msg {
		if feedSvc == nil {
			return digestFinishedMsg{err: fmt.Errorf("feed service is not configured")}
		}
		digest, err := feedSvc.GenerateDigestWithOptions(context.Background(), opts)
		return digestFinishedMsg{digest: digest, err: err}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case syncFinishedMsg:
		m.isBusy = false
		m.busyAction = ""
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("❌ Sync failed: %v", msg.err))
		} else {
			m.setStatus(fmt.Sprintf("✓ Synced %d new posts into local database", msg.count))
			m.reloadChannels()
		}
		return m, nil

	case digestFinishedMsg:
		m.isBusy = false
		m.busyAction = ""
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("❌ Digest failed: %v", msg.err))
		} else {
			m.setStatus(fmt.Sprintf("✓ %s created! (%d posts summarized)", msg.digest.Title, msg.digest.PostCount))
			m.reloadDigests()
			m.reloadChannels()
			m.activeView = ViewDigests
			m.digestCursor = 0
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.promptTextArea.SetWidth(max(40, m.width-6))
		m.promptTextArea.SetHeight(max(10, m.height-10))
		return m, nil

	case tea.KeyMsg:
		// When inside Digest Menu dialog
		if m.showingDigestMenu {
			switch msg.String() {
			case "esc":
				m.showingDigestMenu = false
				m.digestTopicInput.Blur()
				return m, nil
			case "tab", "left", "right":
				m.digestMenuScope = (m.digestMenuScope + 1) % 3
				if m.digestMenuScope == 2 {
					m.digestTopicInput.Focus()
				} else {
					m.digestTopicInput.Blur()
				}
				return m, nil
			case "up", "k":
				if m.digestMenuScope == 2 && msg.String() == "k" {
					var cmd tea.Cmd
					m.digestTopicInput, cmd = m.digestTopicInput.Update(msg)
					return m, cmd
				}
				if m.digestMenuPeriod > 0 {
					m.digestMenuPeriod--
				}
				return m, nil
			case "down", "j":
				if m.digestMenuScope == 2 && msg.String() == "j" {
					var cmd tea.Cmd
					m.digestTopicInput, cmd = m.digestTopicInput.Update(msg)
					return m, cmd
				}
				if m.digestMenuPeriod < 5 {
					m.digestMenuPeriod++
				}
				return m, nil
			case "enter":
				opts := service.DigestOptions{}
				targetDesc := "All Tracked Channels"
				if m.digestMenuScope == 1 && len(m.filteredChannels) > 0 && m.cursor < len(m.filteredChannels) {
					currentCh := m.filteredChannels[m.cursor]
					opts.ChannelID = currentCh.ID
					targetDesc = currentCh.Title
				} else if m.digestMenuScope == 2 {
					topic := strings.TrimSpace(m.digestTopicInput.Value())
					if topic == "" {
						m.setStatus("⚠️ Please enter one or more tags (e.g. 'go + k8s' or 'ai')")
						return m, nil
					}
					opts.Topic = topic
					targetDesc = fmt.Sprintf("Topic: %s", topic)
				}

				m.showingDigestMenu = false
				m.digestTopicInput.Blur()
				if m.isBusy {
					m.setStatus("⚠️ Busy: " + m.busyAction)
					return m, nil
				}

				var periodDesc string
				switch m.digestMenuPeriod {
				case 0:
					opts.TimeRange = service.TimeRangeUnread
					periodDesc = "Unread"
				case 1:
					opts.TimeRange = service.TimeRangeToday
					periodDesc = "Today (24h)"
				case 2:
					opts.TimeRange = service.TimeRangePast7Days
					periodDesc = "Past 7 Days"
				case 3:
					opts.TimeRange = service.TimeRangePast30Days
					periodDesc = "Past 30 Days"
				case 4:
					opts.TimeRange = service.TimeRangeCurrentMonth
					periodDesc = time.Now().Format("January 2006")
				case 5:
					opts.TimeRange = service.TimeRangePrevMonth
					periodDesc = time.Now().AddDate(0, -1, 0).Format("January 2006")
				}

				m.isBusy = true
				m.busyAction = fmt.Sprintf("Generating digest for %s (%s)...", targetDesc, periodDesc)
				m.setStatus(fmt.Sprintf("⏳ Generating digest for [%s] (%s)...", targetDesc, periodDesc))
				return m, generateDigestWithOptionsCmd(m.feedSvc, opts)

			default:
				if m.digestMenuScope == 2 {
					var cmd tea.Cmd
					m.digestTopicInput, cmd = m.digestTopicInput.Update(msg)
					return m, cmd
				}
				if msg.String() == "q" {
					m.showingDigestMenu = false
					m.digestTopicInput.Blur()
					return m, nil
				}
				return m, nil
			}
		}

		// When inside Channel Details view
		if m.showingChannelDetails {
			return m.updateChannelDetails(msg)
		}

		// When typing inside filter input
		if m.filtering {
			switch msg.String() {
			case "enter", "esc":
				m.filtering = false
				m.filterInput.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				m.applyFilter()
				return m, cmd
			}
		}

		// When adding a new ad pattern
		if m.addingPattern {
			switch msg.String() {
			case "enter":
				val := strings.TrimSpace(m.newPatternIn.Value())
				if val != "" {
					ctx := context.Background()
					_ = m.repo.AddAdPattern(ctx, &domain.AdPattern{
						Pattern: val,
						IsRegex: strings.Contains(val, `\`) || strings.Contains(val, ".*") || strings.Contains(val, "("),
						Enabled: true,
					})
					m.reloadSettingsAndPatterns()
					m.setStatus(fmt.Sprintf("Added ad pattern: %s", val))
				}
				m.addingPattern = false
				m.newPatternIn.Reset()
				m.newPatternIn.Blur()
				return m, nil
			case "esc":
				m.addingPattern = false
				m.newPatternIn.Reset()
				m.newPatternIn.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.newPatternIn, cmd = m.newPatternIn.Update(msg)
				return m, cmd
			}
		}

		// When editing AI digest prompt template
		if m.editingPrompt {
			switch msg.String() {
			case "ctrl+s":
				val := m.promptTextArea.Value()
				ctx := context.Background()
				_ = m.repo.SetSetting(ctx, domain.SettingKeyDigestPrompt, val)
				m.digestPrompt = val
				m.editingPrompt = false
				m.promptTextArea.Blur()
				m.reloadSettingsAndPatterns()
				m.setStatus("✓ Digest prompt template saved to database")
				return m, nil
			case "ctrl+r":
				m.promptTextArea.SetValue(domain.DefaultDigestPrompt)
				m.setStatus("Reset prompt to default template (Press ctrl+s to save)")
				return m, nil
			case "esc":
				m.editingPrompt = false
				m.promptTextArea.Blur()
				m.setStatus("Prompt editing cancelled")
				return m, nil
			default:
				var cmd tea.Cmd
				m.promptTextArea, cmd = m.promptTextArea.Update(msg)
				return m, cmd
			}
		}

		// Global navigation keys
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "q":
			if m.editingSetting || m.editingPrompt || m.addingPattern || m.filtering || m.showingDigestMenu || m.showingChannelDetails {
				break
			}
			if m.readingFullDigest {
				m.readingFullDigest = false
				return m, nil
			}
			if m.activeView != ViewChannels {
				m.activeView = ViewChannels
				return m, nil
			}
			return m, tea.Quit

		case "tab":
			if m.editingSetting || m.editingPrompt || m.addingPattern || m.filtering || m.showingDigestMenu || m.readingFullDigest || m.showingChannelDetails {
				break
			}
			switch m.activeView {
			case ViewChannels:
				m.activeView = ViewSettings
			case ViewSettings:
				m.activeView = ViewPatterns
			case ViewPatterns:
				m.activeView = ViewDigests
			default:
				m.activeView = ViewChannels
			}
			return m, nil

		case "o":
			if m.editingSetting || m.editingPrompt || m.addingPattern || m.filtering || m.showingDigestMenu || m.readingFullDigest || m.showingChannelDetails {
				break
			}
			if m.activeView == ViewSettings {
				m.activeView = ViewChannels
			} else {
				m.activeView = ViewSettings
			}
			return m, nil

		case "p":
			if m.editingSetting || m.editingPrompt || m.addingPattern || m.filtering || m.showingDigestMenu || m.readingFullDigest || m.showingChannelDetails {
				break
			}
			if m.activeView == ViewPatterns {
				m.activeView = ViewChannels
			} else {
				m.activeView = ViewPatterns
			}
			return m, nil

		case "d":
			if m.editingSetting || m.editingPrompt || m.addingPattern || m.filtering || m.showingDigestMenu || m.readingFullDigest || m.showingChannelDetails {
				break
			}
			if m.activeView == ViewDigests {
				m.activeView = ViewChannels
			} else {
				m.activeView = ViewDigests
			}
			return m, nil

		case "r":
			if m.editingSetting || m.editingPrompt || m.addingPattern || m.filtering || m.showingDigestMenu || m.readingFullDigest || m.showingChannelDetails {
				break
			}
			m.reloadChannels()
			m.reloadSettingsAndPatterns()
			m.reloadDigests()
			m.setStatus("Refreshed all data from database")
			return m, nil

		case "/":
			if m.activeView == ViewChannels && !m.filtering {
				m.filtering = true
				m.filterInput.Focus()
				return m, nil
			}
		}

		// View-specific actions
		switch m.activeView {
		case ViewChannels:
			return m.updateChannels(msg)
		case ViewSettings:
			return m.updateSettings(msg)
		case ViewPatterns:
			return m.updatePatterns(msg)
		case ViewDigests:
			return m.updateDigests(msg)
		}
	}

	return m, nil
}

func (m *Model) updateChannels(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.filteredChannels) == 0 {
		return m, nil
	}

	ctx := context.Background()
	currentCh := m.filteredChannels[m.cursor]

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.filteredChannels)-1 {
			m.cursor++
		}
	case " ": // Toggle Tracked (SYNC)
		newTracked := !currentCh.IsTracked
		currentCh.IsTracked = newTracked
		_ = m.repo.SetTracking(ctx, []int64{currentCh.ID}, newTracked)
		m.setStatus(fmt.Sprintf("Sync tracking for [%s] set to %v", currentCh.Title, newTracked))

	case "m": // Toggle MCP Allowed (Zero-Trust Privacy Gateway)
		newAllowed := !currentCh.MCPAllowed
		currentCh.MCPAllowed = newAllowed
		_ = m.repo.SetMCPAllowed(ctx, []int64{currentCh.ID}, newAllowed)
		if newAllowed {
			m.setStatus(fmt.Sprintf("Zero-Trust Gateway: [%s] is now VISIBLE to third-party LLMs over MCP", currentCh.Title))
		} else {
			m.setStatus(fmt.Sprintf("Zero-Trust Gateway: [%s] is now QUARANTINED from MCP", currentCh.Title))
		}

	case "a": // Toggle Ad Filter
		newFilter := !currentCh.FilterAds
		currentCh.FilterAds = newFilter
		_ = m.repo.SetFilterAds(ctx, []int64{currentCh.ID}, newFilter)
		m.setStatus(fmt.Sprintf("Ad filter for [%s] set to %v", currentCh.Title, newFilter))

	case "s": // Sync posts from Telegram cloud
		if m.isBusy {
			m.setStatus("⚠️ Busy: " + m.busyAction)
			return m, nil
		}
		m.isBusy = true
		m.busyAction = "Syncing messages from Telegram..."
		m.setStatus("⏳ Syncing new posts from Telegram into local database...")
		return m, syncPostsCmd(m.feedSvc)

	case "g": // Open Digest Creation Menu (default: all channels)
		m.showingDigestMenu = true
		m.digestMenuScope = 0
		m.digestMenuPeriod = 0
		return m, nil

	case "c": // Open Digest Creation Menu targeting current channel
		m.showingDigestMenu = true
		m.digestMenuScope = 1
		m.digestMenuPeriod = 0
		return m, nil

	case "enter", "i": // Open Channel Details view
		m.showingChannelDetails = true
		m.editingChannelTags = false
		return m, nil
	}

	return m, nil
}

func (m *Model) updateChannelDetails(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.filteredChannels) == 0 || m.cursor >= len(m.filteredChannels) {
		m.showingChannelDetails = false
		return m, nil
	}

	currentCh := m.filteredChannels[m.cursor]
	ctx := context.Background()

	if m.editingChannelTags {
		switch msg.String() {
		case "enter":
			val := m.channelTagsInput.Value()
			parts := strings.Split(val, ",")
			var newTopics []string
			seen := make(map[string]bool)
			for _, p := range parts {
				t := strings.TrimSpace(p)
				t = strings.TrimPrefix(t, "#")
				if t != "" && !seen[strings.ToLower(t)] {
					seen[strings.ToLower(t)] = true
					newTopics = append(newTopics, t)
				}
			}
			currentCh.Topics = newTopics
			_ = m.repo.UpdateChannelTopics(ctx, currentCh.ID, newTopics)
			m.editingChannelTags = false
			m.channelTagsInput.Blur()
			m.setStatus(fmt.Sprintf("✓ Saved %d tags for [%s]", len(newTopics), currentCh.Title))
			return m, nil

		case "esc":
			m.editingChannelTags = false
			m.channelTagsInput.Blur()
			return m, nil

		default:
			var cmd tea.Cmd
			m.channelTagsInput, cmd = m.channelTagsInput.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "t", "e":
		m.editingChannelTags = true
		m.channelTagsInput.SetValue(strings.Join(currentCh.Topics, ", "))
		m.channelTagsInput.Focus()
		return m, nil

	case " ": // Toggle tracking
		newTracked := !currentCh.IsTracked
		currentCh.IsTracked = newTracked
		_ = m.repo.SetTracking(ctx, []int64{currentCh.ID}, newTracked)
		m.setStatus(fmt.Sprintf("Sync tracking for [%s] set to %v", currentCh.Title, newTracked))

	case "m": // Toggle MCP
		newAllowed := !currentCh.MCPAllowed
		currentCh.MCPAllowed = newAllowed
		_ = m.repo.SetMCPAllowed(ctx, []int64{currentCh.ID}, newAllowed)
		if newAllowed {
			m.setStatus(fmt.Sprintf("Zero-Trust Gateway: [%s] is now VISIBLE to third-party LLMs over MCP", currentCh.Title))
		} else {
			m.setStatus(fmt.Sprintf("Zero-Trust Gateway: [%s] is now QUARANTINED from MCP", currentCh.Title))
		}

	case "a": // Toggle Ads
		newFilter := !currentCh.FilterAds
		currentCh.FilterAds = newFilter
		_ = m.repo.SetFilterAds(ctx, []int64{currentCh.ID}, newFilter)
		m.setStatus(fmt.Sprintf("Ad filter for [%s] set to %v", currentCh.Title, newFilter))

	case "g":
		m.showingChannelDetails = false
		m.showingDigestMenu = true
		m.digestMenuScope = 0
		m.digestMenuPeriod = 0

	case "c":
		m.showingChannelDetails = false
		m.showingDigestMenu = true
		m.digestMenuScope = 1
		m.digestMenuPeriod = 0

	case "esc", "q", "enter":
		m.showingChannelDetails = false
	}

	return m, nil
}

func (m *Model) getTrackedChannelTags() []string {
	seen := make(map[string]bool)
	var tags []string
	for _, ch := range m.channels {
		for _, t := range ch.Topics {
			trimmed := strings.ToLower(strings.TrimSpace(t))
			if trimmed != "" && !seen[trimmed] {
				seen[trimmed] = true
				tags = append(tags, trimmed)
			}
		}
	}
	sort.Strings(tags)
	return tags
}

func (m *Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.getSettingItems()

	// If currently editing text input for a setting
	if m.editingSetting {
		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(m.settingInput.Value())
			item := items[m.settingsCursor]
			ctx := context.Background()

			if item.Key == domain.SettingKeyGeminiAPIKey {
				valLower := strings.ToLower(val)
				if val == "" || valLower == "no" || valLower == "none" || valLower == "skip" {
					_ = m.repo.SetSetting(ctx, domain.SettingKeyGeminiAPIKey, "")
					m.geminiKey = ""
					m.setStatus("Cleared Gemini API key (Pure MCP mode active)")
				} else {
					// Live validate
					if err := gemini.ValidateAPIKey(ctx, val); err != nil {
						m.setStatus(fmt.Sprintf("❌ Invalid Gemini API key: %v", err))
						return m, nil
					}
					_ = m.repo.SetSetting(ctx, domain.SettingKeyGeminiAPIKey, val)
					m.geminiKey = val
					m.setStatus("✓ Gemini API key verified and saved!")
				}
			} else {
				_ = m.repo.SetSetting(ctx, item.Key, val)
				m.setStatus(fmt.Sprintf("Updated %s to '%s'", item.Label, val))
			}

			m.reloadSettingsAndPatterns()
			m.editingSetting = false
			m.settingInput.Blur()
			m.settingInput.Reset()
			return m, nil

		case "esc":
			m.editingSetting = false
			m.settingInput.Blur()
			m.settingInput.Reset()
			return m, nil

		default:
			var cmd tea.Cmd
			m.settingInput, cmd = m.settingInput.Update(msg)
			return m, cmd
		}
	}

	// Normal navigation in settings list
	switch msg.String() {
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case "down", "j":
		if m.settingsCursor < len(items)-1 {
			m.settingsCursor++
		}
	case " ": // Toggle or cycle
		item := items[m.settingsCursor]
		ctx := context.Background()
		if item.IsToggle {
			newVal := "true"
			if item.Value == "true" {
				newVal = "false"
			}
			_ = m.repo.SetSetting(ctx, item.Key, newVal)
			m.reloadSettingsAndPatterns()
			m.setStatus(fmt.Sprintf("Set %s: %s", item.Label, newVal))
		} else if item.IsCycle {
			currentIdx := 0
			for idx, opt := range item.CycleOpts {
				if opt == item.Value {
					currentIdx = idx
					break
				}
			}
			nextIdx := (currentIdx + 1) % len(item.CycleOpts)
			nextOpt := item.CycleOpts[nextIdx]
			_ = m.repo.SetSetting(ctx, item.Key, nextOpt)
			m.reloadSettingsAndPatterns()
			m.setStatus(fmt.Sprintf("Switched model to %s", nextOpt))
		}
	case "enter": // Open text edit
		item := items[m.settingsCursor]
		if item.Key == domain.SettingKeyDigestPrompt {
			m.editingPrompt = true
			curVal := m.digestPrompt
			if strings.TrimSpace(curVal) == "" {
				curVal = domain.DefaultDigestPrompt
			}
			m.promptTextArea.SetWidth(max(40, m.width-6))
			m.promptTextArea.SetHeight(max(10, m.height-10))
			m.promptTextArea.SetValue(curVal)
			m.promptTextArea.Focus()
			return m, nil
		}
		if !item.IsToggle && !item.IsCycle {
			m.editingSetting = true
			m.settingInput.Prompt = fmt.Sprintf("Edit %s: ", item.Label)
			curVal := ""
			switch item.Key {
			case domain.SettingKeyBotToken:
				curVal = m.botToken
			case domain.SettingKeyBotChatID:
				curVal = m.botChatID
			case domain.SettingKeyCronSchedule:
				curVal = m.cronSchedule
			case domain.SettingKeyMinPostLength:
				curVal = m.minPostLength
			case domain.SettingKeyUserInterests:
				curVal = m.userInterests
			case domain.SettingKeyUserIgnoredTopics:
				curVal = m.userIgnoredTopics
			case domain.SettingKeyGeminiAPIKey:
				curVal = ""
			}
			m.settingInput.SetValue(curVal)
			m.settingInput.Focus()
		}
	case "esc":
		m.activeView = ViewChannels
	}

	return m, nil
}

func (m *Model) updatePatterns(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.patternCursor > 0 {
			m.patternCursor--
		}
	case "down", "j":
		if m.patternCursor < len(m.adPatterns)-1 {
			m.patternCursor++
		}
	case " ": // Toggle pattern enabled
		if len(m.adPatterns) > 0 && m.patternCursor < len(m.adPatterns) {
			p := m.adPatterns[m.patternCursor]
			newVal := !p.Enabled
			p.Enabled = newVal
			ctx := context.Background()
			_ = m.repo.ToggleAdPattern(ctx, p.ID, newVal)
			m.setStatus(fmt.Sprintf("Pattern [%s] enabled: %v", p.Pattern, newVal))
		}
	case "+": // Add pattern
		m.addingPattern = true
		m.newPatternIn.Focus()
	case "x", "delete": // Delete pattern
		if len(m.adPatterns) > 0 && m.patternCursor < len(m.adPatterns) {
			p := m.adPatterns[m.patternCursor]
			ctx := context.Background()
			_ = m.repo.DeleteAdPattern(ctx, p.ID)
			m.setStatus(fmt.Sprintf("Deleted ad pattern: %s", p.Pattern))
			m.reloadSettingsAndPatterns()
			if m.patternCursor >= len(m.adPatterns) {
				m.patternCursor = max(0, len(m.adPatterns)-1)
			}
		}
	case "esc":
		m.activeView = ViewChannels
	}
	return m, nil
}

func (m *Model) updateDigests(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.readingFullDigest {
		if len(m.digests) == 0 || m.digestCursor >= len(m.digests) {
			m.readingFullDigest = false
			return m, nil
		}
		selected := m.digests[m.digestCursor]
		lines := strings.Split(selected.SummaryMarkdown, "\n")
		pageSize := max(5, m.height-6)

		switch msg.String() {
		case "up", "k":
			if m.digestScrollOffset > 0 {
				m.digestScrollOffset--
			}
		case "down", "j":
			if m.digestScrollOffset < max(0, len(lines)-pageSize) {
				m.digestScrollOffset++
			}
		case "pgup", "b", "ctrl+u":
			m.digestScrollOffset = max(0, m.digestScrollOffset-pageSize)
		case "pgdown", "f", "ctrl+d", " ":
			m.digestScrollOffset = min(max(0, len(lines)-pageSize), m.digestScrollOffset+pageSize)
		case "home":
			m.digestScrollOffset = 0
		case "end":
			m.digestScrollOffset = max(0, len(lines)-pageSize)
		case "esc", "q", "enter":
			m.readingFullDigest = false
		}
		return m, nil
	}

	switch msg.String() {
	case "up", "k":
		if m.digestCursor > 0 {
			m.digestCursor--
		}
	case "down", "j":
		if m.digestCursor < len(m.digests)-1 {
			m.digestCursor++
		}
	case "enter", " ": // Open Full Digest Reader
		if len(m.digests) > 0 && m.digestCursor < len(m.digests) {
			m.readingFullDigest = true
			m.digestScrollOffset = 0
		}
		return m, nil
	case "x", "delete": // Delete selected digest
		if len(m.digests) > 0 && m.digestCursor < len(m.digests) {
			d := m.digests[m.digestCursor]
			ctx := context.Background()
			_ = m.repo.DeleteDigest(ctx, d.ID)
			m.setStatus(fmt.Sprintf("Deleted digest: %s", d.Title))
			m.reloadDigests()
			if m.digestCursor >= len(m.digests) {
				m.digestCursor = max(0, len(m.digests)-1)
			}
		}
		return m, nil
	case "g": // Open Digest Creation Menu
		m.showingDigestMenu = true
		m.digestMenuScope = 0
		m.digestMenuPeriod = 0
		return m, nil
	case "esc":
		m.activeView = ViewChannels
	}
	return m, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
