package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"telegram_feed_summarizer/internal/domain"
	"telegram_feed_summarizer/internal/llm/gemini"
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

	// Dynamic Ad Patterns
	adPatterns    []*domain.AdPattern
	patternCursor int
	newPatternIn  textinput.Model
	addingPattern bool

	// Digests
	digests      []*domain.Digest
	digestCursor int

	// Status
	statusMsg  string
	statusTime time.Time
	width      int
	height     int
}

// NewModel creates an initialized TUI model.
func NewModel(repo domain.Repository) *Model {
	ti := textinput.New()
	ti.Placeholder = "type to filter channels by title, username or topic..."
	ti.Prompt = "/ "

	np := textinput.New()
	np.Placeholder = "new keyword or regex (e.g. 'скидка' or 't\\.me/\\+.*')..."
	np.Prompt = "+ "

	si := textinput.New()
	si.Prompt = "Enter value: "

	m := &Model{
		repo:         repo,
		activeView:   ViewChannels,
		filterInput:  ti,
		newPatternIn: np,
		settingInput: si,
		width:        100,
		height:       30,
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

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
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

		// Global navigation keys
		switch msg.String() {
		case "ctrl+c", "q":
			if m.editingSetting || m.addingPattern || m.filtering {
				break
			}
			if m.activeView != ViewChannels {
				m.activeView = ViewChannels
				return m, nil
			}
			return m, tea.Quit

		case "tab":
			if m.editingSetting || m.addingPattern || m.filtering {
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
			if m.editingSetting || m.addingPattern || m.filtering {
				break
			}
			if m.activeView == ViewSettings {
				m.activeView = ViewChannels
			} else {
				m.activeView = ViewSettings
			}
			return m, nil

		case "p":
			if m.editingSetting || m.addingPattern || m.filtering {
				break
			}
			if m.activeView == ViewPatterns {
				m.activeView = ViewChannels
			} else {
				m.activeView = ViewPatterns
			}
			return m, nil

		case "d":
			if m.editingSetting || m.addingPattern || m.filtering {
				break
			}
			if m.activeView == ViewDigests {
				m.activeView = ViewChannels
			} else {
				m.activeView = ViewDigests
			}
			return m, nil

		case "r":
			if m.editingSetting || m.addingPattern || m.filtering {
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
	}

	return m, nil
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
	switch msg.String() {
	case "up", "k":
		if m.digestCursor > 0 {
			m.digestCursor--
		}
	case "down", "j":
		if m.digestCursor < len(m.digests)-1 {
			m.digestCursor++
		}
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
