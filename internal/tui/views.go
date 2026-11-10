package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// View renders the currently active view mode.
func (m *Model) View() string {
	var b strings.Builder

	// Top Title Bar - stretches across full terminal width
	title := titleStyle.Render(" 📡 TELEGRAM FEED SUMMARIZER ")
	var tabs string
	switch m.activeView {
	case ViewChannels:
		tabs = " [Channels (Active)]  |  [o] Settings  |  [p] Ad Rules  |  [d] Digests "
	case ViewSettings:
		tabs = "  Channels  |  [o] Settings (Active)  |  [p] Ad Rules  |  [d] Digests "
	case ViewPatterns:
		tabs = "  Channels  |  [o] Settings  |  [p] Ad Rules (Active)  |  [d] Digests "
	case ViewDigests:
		tabs = "  Channels  |  [o] Settings  |  [p] Ad Rules  |  [d] Digests (Active) "
	}

	tabsW := max(0, m.width-lipgloss.Width(title))
	tabsStyle := headerStyle.Width(tabsW)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, title, tabsStyle.Render(tabs)))
	b.WriteString("\n\n")

	// Main View Body
	switch m.activeView {
	case ViewChannels:
		b.WriteString(m.renderChannelsView())
	case ViewSettings:
		b.WriteString(m.renderSettingsView())
	case ViewPatterns:
		b.WriteString(m.renderPatternsView())
	case ViewDigests:
		b.WriteString(m.renderDigestsView())
	}

	// Status Message
	b.WriteString("\n")
	if m.statusMsg != "" && time.Since(m.statusTime) < 5*time.Second {
		b.WriteString(statusBarStyle.Width(m.width).Render("ℹ " + m.statusMsg))
		b.WriteString("\n")
	}

	// Help Bar - stretches across full width
	b.WriteString(m.renderHelpBar())

	return b.String()
}

func (m *Model) getChannelColumnWidths() (titleWidth int, topicsWidth int) {
	totalWidth := m.width
	if totalWidth <= 0 {
		totalWidth = 100
	}

	fixedWidth := 3 + 8 + 7 + 9 // prefix(3) + SYNC(8) + LLM(7) + NO-AD(9) = 27

	topicsWidth = 25
	if totalWidth >= 140 {
		topicsWidth = 35
	} else if totalWidth < 90 {
		topicsWidth = 18
	}

	// Remaining space dynamically given to channel title
	titleWidth = totalWidth - fixedWidth - topicsWidth - 3
	if titleWidth < 25 {
		titleWidth = 25
	}
	return titleWidth, topicsWidth
}

func truncateRunes(s string, maxLen int) string {
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(r[:maxLen])
	}
	return string(r[:maxLen-3]) + "..."
}

func (m *Model) renderChannelsView() string {
	var b strings.Builder

	// Filter Input (if active)
	if m.filtering || m.filterInput.Value() != "" {
		b.WriteString(" " + m.filterInput.View() + "\n\n")
	}

	if len(m.filteredChannels) == 0 {
		b.WriteString("  No channels found. Run 'telegram-feed sync' to fetch your Telegram channels.\n")
		return b.String()
	}

	titleW, topicsW := m.getChannelColumnWidths()
	colTitleDynamic := lipgloss.NewStyle().Width(titleW)
	colTopicsDynamic := lipgloss.NewStyle().Width(topicsW)

	// Table Header - strictly aligned to full terminal width
	headerLine := headerStyle.Width(m.width).Render(
		colPrefixStyle.Render(" ") +
			colSyncStyle.Render("SYNC") +
			colLLMStyle.Render("LLM") +
			colAdStyle.Render("NO-AD") +
			colTitleDynamic.Render("CHANNEL TITLE") +
			colTopicsDynamic.Render("TOPICS"),
	)
	b.WriteString(headerLine + "\n")

	// Render rows
	start := max(0, m.cursor-10)
	end := min(len(m.filteredChannels), start+20)

	for i := start; i < end; i++ {
		ch := m.filteredChannels[i]

		title := truncateRunes(ch.Title, titleW-2)
		topics := truncateRunes(strings.Join(ch.Topics, ", "), topicsW-2)
		if topics == "" {
			topics = "-"
		}

		isSelected := (i == m.cursor)

		var syncStr, llmStr, adStr string
		if ch.IsTracked {
			syncStr = "[x]"
		} else {
			syncStr = "[ ]"
		}
		if ch.MCPAllowed {
			llmStr = "[x]"
		} else {
			llmStr = "[ ]"
		}
		if ch.FilterAds {
			adStr = "[x]"
		} else {
			adStr = "[ ]"
		}

		if isSelected {
			prefix := "> "
			rowText := colPrefixStyle.Render(prefix) +
				colSyncStyle.Render(syncStr) +
				colLLMStyle.Render(llmStr) +
				colAdStyle.Render(adStr) +
				colTitleDynamic.Render(title) +
				colTopicsDynamic.Render(topics)
			b.WriteString(selectedRowStyle.Width(m.width).Render(rowText) + "\n")
		} else {
			prefix := "  "
			var syncStyled, llmStyled, adStyled string
			if ch.IsTracked {
				syncStyled = activeTagStyle.Render(syncStr)
			} else {
				syncStyled = inactiveTagStyle.Render(syncStr)
			}
			if ch.MCPAllowed {
				llmStyled = activeTagStyle.Render(llmStr)
			} else {
				llmStyled = inactiveTagStyle.Render(llmStr)
			}
			if ch.FilterAds {
				adStyled = activeTagStyle.Render(adStr)
			} else {
				adStyled = inactiveTagStyle.Render(adStr)
			}

			rowText := colPrefixStyle.Render(prefix) +
				colSyncStyle.Render(syncStyled) +
				colLLMStyle.Render(llmStyled) +
				colAdStyle.Render(adStyled) +
				colTitleDynamic.Render(title) +
				colTopicsDynamic.Render(topics)
			b.WriteString(rowText + "\n")
		}
	}

	return b.String()
}

func (m *Model) renderSettingsView() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Render(" ⚙ APPLICATION SETTINGS") + "\n")
	b.WriteString(" Press [↑/↓] to navigate, [Space] to toggle/cycle, [Enter] to edit value, [Esc] to exit.\n\n")

	items := m.getSettingItems()

	for i, item := range items {
		isSelected := (i == m.settingsCursor)

		prefix := "  "
		if isSelected {
			prefix = "> "
		}

		valDisplay := item.Value
		if item.IsToggle {
			if item.Value == "true" {
				valDisplay = activeTagStyle.Render("[ENABLED]")
			} else {
				valDisplay = inactiveTagStyle.Render("[DISABLED]")
			}
		} else if item.IsCycle {
			valDisplay = highlightColorRender(item.Value)
		}

		descW := m.width - 3 - 30 - 26 - 3
		if descW < 20 {
			descW = 20
		}

		labelCol := lipgloss.NewStyle().Width(30).Bold(isSelected).Render(item.Label)
		valCol := lipgloss.NewStyle().Width(26).Render(valDisplay)
		descCol := lipgloss.NewStyle().Width(descW).Foreground(lipgloss.Color("#888888")).Render(item.Description)

		row := fmt.Sprintf("%s%s %s %s", prefix, labelCol, valCol, descCol)
		if isSelected {
			b.WriteString(selectedRowStyle.Width(m.width).Render(row) + "\n")
		} else {
			b.WriteString(row + "\n")
		}

		if m.editingSetting && isSelected {
			b.WriteString("\n     " + highlightColorRender("Editing: ") + m.settingInput.View() + "\n")
			b.WriteString("     " + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("(Press [Enter] to save, [Esc] to cancel)\n\n"))
		}
	}

	// Bottom credentials summary
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("  • Telegram App ID: ") + m.telegramAppID)
	hasHash := "Configured"
	if !m.hasAppHash {
		hasHash = "Not set"
	}
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("  |  Telegram App Hash: ") + hasHash + "\n")

	return b.String()
}

func (m *Model) renderPatternsView() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Render(" 🛡️ DYNAMIC NOISE & AD FILTER PATTERNS") + "\n")
	b.WriteString(" Posts matching enabled rules are automatically flagged as 'is_ad' and omitted from digests.\n")
	b.WriteString(" Press [Space] to toggle rule, [+] to add keyword/regex, [x] to delete, [Esc] to exit.\n\n")

	if m.addingPattern {
		b.WriteString(" " + m.newPatternIn.View() + "\n")
		b.WriteString(" " + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("(Press [Enter] to save rule, [Esc] to cancel)") + "\n\n")
	}

	if len(m.adPatterns) == 0 {
		b.WriteString("  No ad patterns defined. Press '+' to add a new rule.\n")
		return b.String()
	}

	descW := 25
	if m.width >= 140 {
		descW = 35
	}
	patW := m.width - 21 - descW - 3
	if patW < 35 {
		patW = 35
	}

	colPat := lipgloss.NewStyle().Width(patW)
	colDesc := lipgloss.NewStyle().Width(descW)

	headerLine := headerStyle.Width(m.width).Render(
		colPrefixStyle.Render(" ") +
			lipgloss.NewStyle().Width(8).Render("ACTIVE") +
			lipgloss.NewStyle().Width(10).Render("TYPE") +
			colPat.Render("PATTERN") +
			colDesc.Render("DESCRIPTION"),
	)
	b.WriteString(headerLine + "\n")

	for i, p := range m.adPatterns {
		isSelected := (i == m.patternCursor)

		prefix := "  "
		if isSelected {
			prefix = "> "
		}

		activeMark := "[ ]"
		if p.Enabled {
			activeMark = "[x]"
		}

		patType := "Keyword"
		if p.IsRegex {
			patType = "Regex"
		}

		pat := truncateRunes(p.Pattern, patW-2)
		desc := truncateRunes(p.Description, descW-2)

		activeCell := lipgloss.NewStyle().Width(8).Render(activeMark)
		typeCell := lipgloss.NewStyle().Width(10).Render(patType)
		patCell := colPat.Render(pat)
		descCell := colDesc.Render(desc)

		rowText := colPrefixStyle.Render(prefix) + activeCell + typeCell + patCell + descCell

		if isSelected {
			b.WriteString(selectedRowStyle.Width(m.width).Render(rowText) + "\n")
		} else {
			if p.Enabled {
				activeCell = lipgloss.NewStyle().Width(8).Render(activeTagStyle.Render("[x]"))
			} else {
				activeCell = lipgloss.NewStyle().Width(8).Render(inactiveTagStyle.Render("[ ]"))
			}
			if p.IsRegex {
				typeCell = lipgloss.NewStyle().Width(10).Render(highlightColorRender("Regex"))
			}
			rowText = colPrefixStyle.Render(prefix) + activeCell + typeCell + patCell + descCell
			b.WriteString(rowText + "\n")
		}
	}

	return b.String()
}

func (m *Model) renderDigestsView() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Render(" 📑 PAST DIGEST HISTORY (Fast Recall Memory)") + "\n")
	b.WriteString(" Third-party LLMs can query these past digests directly over MCP without reading raw messages.\n\n")

	if len(m.digests) == 0 {
		b.WriteString("  No digests generated yet. Generate one with 'telegram-feed digest' or via your LLM chat.\n")
		return b.String()
	}

	header := fmt.Sprintf("  %-25s %-12s %-10s %-12s", "DATE", "TOPIC", "POSTS", "MCP VISIBLE")
	b.WriteString(headerStyle.Width(m.width).Render(header) + "\n")

	for i, d := range m.digests {
		mcpVis := inactiveTagStyle.Render("Quarantined")
		if d.MCPVisible {
			mcpVis = activeTagStyle.Render("Allowed")
		}

		prefix := "  "
		if i == m.digestCursor {
			prefix = "> "
		}

		dateStr := d.CreatedAt.Format("2006-01-02 15:04")
		row := fmt.Sprintf("%s%-25s %-12s %-10d %-12s", prefix, dateStr, d.Topic, d.PostCount, mcpVis)

		if i == m.digestCursor {
			b.WriteString(selectedRowStyle.Width(m.width).Render(row) + "\n")
		} else {
			b.WriteString(row + "\n")
		}
	}

	// Preview selected digest
	if len(m.digests) > 0 && m.digestCursor < len(m.digests) {
		selected := m.digests[m.digestCursor]
		b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render(" ── Summary Preview: "+selected.Title+" ──") + "\n")
		lines := strings.Split(selected.SummaryMarkdown, "\n")
		for j := 0; j < min(len(lines), 10); j++ {
			b.WriteString("  " + lines[j] + "\n")
		}
	}

	return b.String()
}

func (m *Model) renderHelpBar() string {
	var keys []string

	switch m.activeView {
	case ViewChannels:
		keys = []string{
			helpKey("[Space]") + " Track",
			helpKey("[m]") + " LLM Access",
			helpKey("[a]") + " Ad Filter",
			helpKey("[/]") + " Search",
			helpKey("[o]") + " Settings",
			helpKey("[p]") + " Ad Rules",
			helpKey("[d]") + " Digests",
			helpKey("[r]") + " Refresh",
			helpKey("[q]") + " Quit",
		}
	case ViewSettings:
		keys = []string{
			helpKey("[↑/↓]") + " Navigate",
			helpKey("[Space]") + " Toggle/Cycle",
			helpKey("[Enter]") + " Edit Value",
			helpKey("[Esc]") + " Back",
		}
	case ViewPatterns:
		keys = []string{
			helpKey("[Space]") + " Toggle Rule",
			helpKey("[+]") + " Add Rule",
			helpKey("[x]") + " Delete Rule",
			helpKey("[Esc]") + " Back",
		}
	case ViewDigests:
		keys = []string{
			helpKey("[↑/↓]") + " Select Digest",
			helpKey("[Esc]") + " Back",
		}
	}

	return statusBarStyle.Width(m.width).Render(strings.Join(keys, "  "))
}

func helpKey(k string) string {
	return helpKeyStyle.Render(k)
}

func highlightColorRender(s string) string {
	return lipgloss.NewStyle().Foreground(highlightColor).Render(s)
}

func dangerColorRender(s string) string {
	return lipgloss.NewStyle().Foreground(dangerColor).Render(s)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
