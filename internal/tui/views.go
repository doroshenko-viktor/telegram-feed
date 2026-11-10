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
	if m.showingDigestMenu {
		b.WriteString(m.renderDigestMenuModal())
	} else if m.readingFullDigest {
		b.WriteString(m.renderFullDigestReader())
	} else {
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
	}

	// Status Message
	var statusLine string
	if m.isBusy {
		statusLine = statusBarStyle.Width(m.width).Render("⏳ " + m.busyAction)
	} else if m.statusMsg != "" {
		timeout := 5 * time.Second
		if strings.HasPrefix(m.statusMsg, "❌") || strings.HasPrefix(m.statusMsg, "⚠️") {
			timeout = 25 * time.Second
		}
		if time.Since(m.statusTime) < timeout {
			statusLine = statusBarStyle.Width(m.width).Render("ℹ " + m.statusMsg)
		}
	}

	helpBar := m.renderHelpBar()

	// Fill remaining vertical height so help bar is pinned to the absolute bottom line of the terminal (100% height)
	rendered := b.String()
	lineCount := strings.Count(rendered, "\n")
	bottomLines := 1 // helpBar
	if statusLine != "" {
		bottomLines += 2 // newline + statusLine
	} else {
		bottomLines += 1 // newline before helpBar
	}

	padLines := m.height - lineCount - bottomLines
	if padLines > 0 {
		rendered += strings.Repeat("\n", padLines)
	}

	if statusLine != "" {
		rendered += "\n" + statusLine
	}
	rendered += "\n" + helpBar

	return rendered
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
	if m.showingChannelDetails {
		return m.renderChannelDetailsView()
	}

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

	statsInfo := fmt.Sprintf(" 💾 Local Storage: %d posts cached (%d unread)  •  [s] Sync Telegram  •  [g] Digest Menu  •  [c] Channel Digest",
		m.totalPosts, m.unreadPosts)
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#8E8EA0")).Render(statsInfo) + "\n\n")

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

	// Dynamic vertical height calculation to fill available screen
	overhead := 10
	if m.filtering || m.filterInput.Value() != "" {
		overhead += 2
	}
	tableHeight := max(5, m.height-overhead)

	// Keep cursor within visible window
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+tableHeight {
		m.scrollOffset = m.cursor - tableHeight + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	start := m.scrollOffset
	end := min(len(m.filteredChannels), start+tableHeight)

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

	// Selected channel footer info
	if len(m.filteredChannels) > 0 && m.cursor < len(m.filteredChannels) {
		selectedCh := m.filteredChannels[m.cursor]
		username := selectedCh.Username
		if username == "" {
			username = "private"
		} else {
			username = "@" + username
		}
		detailLine := fmt.Sprintf(" ── Channel [%d/%d]: %s (%s) • Watermark: Telegram Msg #%d ──",
			m.cursor+1, len(m.filteredChannels), truncateRunes(selectedCh.Title, 40), username, selectedCh.LastSyncMsgID)
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(subtleColor).Render(detailLine) + "\n")
	}

	return b.String()
}

func (m *Model) renderChannelDetailsView() string {
	if len(m.filteredChannels) == 0 || m.cursor >= len(m.filteredChannels) {
		return "No channel selected."
	}
	ch := m.filteredChannels[m.cursor]

	var b strings.Builder

	boxWidth := min(86, m.width-4)
	if boxWidth < 50 {
		boxWidth = 50
	}

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 2).
		Width(boxWidth)

	var content strings.Builder
	titleStr := fmt.Sprintf("📢 CHANNEL DETAILS: %s", ch.Title)
	if ch.Username != "" {
		titleStr += fmt.Sprintf(" (@%s)", ch.Username)
	}
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(highlightColor).Render(titleStr) + "\n")
	content.WriteString(lipgloss.NewStyle().Foreground(subtleColor).Render(strings.Repeat("─", boxWidth-6)) + "\n\n")

	// Badges
	syncBadge := inactiveTagStyle.Render("[DISABLED]")
	if ch.IsTracked {
		syncBadge = activeTagStyle.Render("[ENABLED]")
	}
	mcpBadge := inactiveTagStyle.Render("[QUARANTINED]")
	if ch.MCPAllowed {
		mcpBadge = activeTagStyle.Render("[ALLOWED]")
	}
	adBadge := inactiveTagStyle.Render("[DISABLED]")
	if ch.FilterAds {
		adBadge = activeTagStyle.Render("[ENABLED]")
	}

	lblStyle := lipgloss.NewStyle().Bold(true).Width(16)
	content.WriteString(fmt.Sprintf(" %s %s\n", lblStyle.Render("Channel ID:"), fmt.Sprintf("%d", ch.ID)))
	if ch.Username != "" {
		content.WriteString(fmt.Sprintf(" %s %s\n", lblStyle.Render("Telegram Link:"), fmt.Sprintf("https://t.me/%s", ch.Username)))
	}
	content.WriteString(fmt.Sprintf(" %s %s  %s\n", lblStyle.Render("Sync Tracking:"), syncBadge, lipgloss.NewStyle().Foreground(subtleColor).Render("(Space to toggle)")))
	content.WriteString(fmt.Sprintf(" %s %s  %s\n", lblStyle.Render("MCP LLM Access:"), mcpBadge, lipgloss.NewStyle().Foreground(subtleColor).Render("(m to toggle)")))
	content.WriteString(fmt.Sprintf(" %s %s  %s\n", lblStyle.Render("Ad Filtering:"), adBadge, lipgloss.NewStyle().Foreground(subtleColor).Render("(a to toggle)")))

	cType := ch.ContentType
	if cType == "" {
		cType = "general"
	}
	lang := ch.Language
	if lang == "" {
		lang = "en"
	}
	content.WriteString(fmt.Sprintf(" %s %s  •  Language: %s\n", lblStyle.Render("Content Type:"), cType, lang))

	if ch.About != "" {
		content.WriteString(fmt.Sprintf("\n %s\n   %s\n", lblStyle.Render("Bio / About:"), lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC")).Render(ch.About)))
	}
	if ch.Summary != "" {
		content.WriteString(fmt.Sprintf("\n %s\n   %s\n", lblStyle.Render("AI Summary:"), lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC")).Render(ch.Summary)))
	}

	content.WriteString("\n" + lipgloss.NewStyle().Foreground(subtleColor).Render(strings.Repeat("─", boxWidth-6)) + "\n")
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00D7AF")).Render("🏷️ TOPIC TAGS:") + "\n")

	if m.editingChannelTags {
		content.WriteString(" " + m.channelTagsInput.View() + "\n")
		content.WriteString(" " + lipgloss.NewStyle().Foreground(subtleColor).Render("(Press [Enter] to save tags, [Esc] to cancel)\n"))
	} else {
		if len(ch.Topics) > 0 {
			var tagBadges []string
			for _, t := range ch.Topics {
				tagBadges = append(tagBadges, lipgloss.NewStyle().
					Background(lipgloss.Color("#332255")).
					Foreground(lipgloss.Color("#DDAAEE")).
					Padding(0, 1).
					Render("#"+t))
			}
			content.WriteString("   " + strings.Join(tagBadges, "  ") + "\n\n")
		} else {
			content.WriteString("   " + lipgloss.NewStyle().Foreground(subtleColor).Render("No tags assigned yet.") + "\n\n")
		}
		content.WriteString("   " + lipgloss.NewStyle().Foreground(subtleColor).Render("Press [t] to edit/add tags (comma-separated, e.g. 'go, programming, ai')\n"))
	}

	content.WriteString("\n" + lipgloss.NewStyle().Foreground(subtleColor).Render("Controls: [t] Edit Tags  •  [Space] Sync  •  [m] MCP  •  [a] Ads  •  [Esc] Back to Channels"))

	b.WriteString(borderStyle.Render(content.String()))
	return b.String()
}

func (m *Model) renderSettingsView() string {
	if m.editingPrompt {
		return m.renderPromptEditorView()
	}

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

func (m *Model) renderPromptEditorView() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00D7AF")).Render(" 🤖 EDIT AI DIGEST PROMPT TEMPLATE") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(
		" Controls instructions given to the LLM when summarizing posts. Use [{{topic}}] for subject placeholder.\n"+
			" Keys: [ctrl+s] Save to DB  •  [ctrl+r] Reset to Default Template  •  [Esc] Cancel\n\n",
	))

	contentBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Render(m.promptTextArea.View())

	b.WriteString(contentBox + "\n")
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
		b.WriteString("  No digests generated yet. Press [g] to generate a digest from your stored posts.\n")
		return b.String()
	}

	header := fmt.Sprintf("  %-25s %-12s %-10s %-12s", "DATE", "TOPIC", "POSTS", "MCP VISIBLE")
	b.WriteString(headerStyle.Width(m.width).Render(header) + "\n")

	maxDigestList := min(len(m.digests), 8)
	for i := 0; i < maxDigestList; i++ {
		d := m.digests[i]
		mcpVis := inactiveTagStyle.Render("Quarantined")
		if d.MCPVisible {
			mcpVis = activeTagStyle.Render("Allowed")
		}

		prefix := "  "
		if i == m.digestCursor {
			prefix = "> "
		}

		dateStr := d.CreatedAt.Format("2006-01-02 15:04")
		row := fmt.Sprintf("%s%-25s %-12s %-10d %-12s", prefix, dateStr, truncateRunes(d.Topic, 12), d.PostCount, mcpVis)

		if i == m.digestCursor {
			b.WriteString(selectedRowStyle.Width(m.width).Render(row) + "\n")
		} else {
			b.WriteString(row + "\n")
		}
	}

	// Preview selected digest - dynamically expands to fill remaining screen height
	if len(m.digests) > 0 && m.digestCursor < len(m.digests) {
		selected := m.digests[m.digestCursor]
		b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render(" ── Summary Preview: "+selected.Title+" (Press [Enter] to read full digest) ──") + "\n")
		contentWidth := max(30, m.width-6)
		lines := wrapMarkdownLines(selected.SummaryMarkdown, contentWidth)
		previewCapacity := max(6, m.height-maxDigestList-14)
		for j := 0; j < min(len(lines), previewCapacity); j++ {
			b.WriteString("  " + lines[j] + "\n")
		}
		if len(lines) > previewCapacity {
			b.WriteString(lipgloss.NewStyle().Foreground(subtleColor).Render(
				fmt.Sprintf("  *... and %d more lines (Press [Enter] to read all)*", len(lines)-previewCapacity)) + "\n")
		}
	}

	return b.String()
}

func wrapMarkdownLines(markdown string, width int) []string {
	if width < 30 {
		width = 30
	}
	rawLines := strings.Split(markdown, "\n")
	var result []string

	for _, rawLine := range rawLines {
		trimmed := strings.TrimRight(rawLine, " \t\r")
		if trimmed == "" {
			result = append(result, "")
			continue
		}

		if strings.HasPrefix(trimmed, "# ") {
			rendered := lipgloss.NewStyle().Width(width).Bold(true).Foreground(highlightColor).Render(trimmed)
			for _, sub := range strings.Split(rendered, "\n") {
				result = append(result, sub)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			rendered := lipgloss.NewStyle().Width(width).Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render(trimmed)
			for _, sub := range strings.Split(rendered, "\n") {
				result = append(result, sub)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			rendered := lipgloss.NewStyle().Width(width).Bold(true).Foreground(accentColor).Render(trimmed)
			for _, sub := range strings.Split(rendered, "\n") {
				result = append(result, sub)
			}
			continue
		}

		// Style and wrap normal markdown lines
		rendered := lipgloss.NewStyle().Width(width).Render(trimmed)
		for _, sub := range strings.Split(rendered, "\n") {
			result = append(result, sub)
		}
	}

	return result
}

func (m *Model) renderFullDigestReader() string {
	var b strings.Builder
	if len(m.digests) == 0 || m.digestCursor >= len(m.digests) {
		return "  No digest selected.\n"
	}
	selected := m.digests[m.digestCursor]

	// Header line
	headerInfo := fmt.Sprintf(" 📖 %s  •  %s  (%d posts)",
		selected.Title,
		selected.CreatedAt.Format("2006-01-02 15:04"),
		selected.PostCount,
	)
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(highlightColor).Render(headerInfo) + "\n\n")

	contentWidth := max(30, m.width-4)
	lines := wrapMarkdownLines(selected.SummaryMarkdown, contentWidth)
	pageSize := max(5, m.height-6)

	maxOffset := max(0, len(lines)-pageSize)
	if m.digestScrollOffset > maxOffset {
		m.digestScrollOffset = maxOffset
	}

	start := m.digestScrollOffset
	end := min(len(lines), start+pageSize)

	for i := start; i < end; i++ {
		b.WriteString(lines[i] + "\n")
	}

	return b.String()
}

func (m *Model) renderDigestMenuModal() string {
	var b strings.Builder

	selectedChTitle := "No channel selected"
	var selectedChUsername string
	if len(m.filteredChannels) > 0 && m.cursor < len(m.filteredChannels) {
		ch := m.filteredChannels[m.cursor]
		selectedChTitle = ch.Title
		selectedChUsername = ch.Username
	}

	boxWidth := min(76, m.width-4)
	if boxWidth < 45 {
		boxWidth = 45
	}

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 2).
		Width(boxWidth)

	var content strings.Builder
	titleLine := lipgloss.NewStyle().Bold(true).Foreground(highlightColor).Render("📝 CREATE FEED DIGEST")
	content.WriteString(titleLine + "\n")
	content.WriteString(lipgloss.NewStyle().Foreground(subtleColor).Render("Generate an AI executive summary or extractive report from stored posts.") + "\n\n")

	// Section 1: Target Scope
	content.WriteString(lipgloss.NewStyle().Bold(true).Render("1. Target Scope:") + "\n")
	scope0 := "[ ] All Tracked Channels (Global Feed)"
	if m.digestMenuScope == 0 {
		scope0 = activeTagStyle.Render("[•] All Tracked Channels (Global Feed)")
	}
	content.WriteString("   " + scope0 + "\n")

	chDesc := selectedChTitle
	if selectedChUsername != "" {
		chDesc += " (@" + selectedChUsername + ")"
	}
	scope1 := "[ ] Individual Channel: " + chDesc
	if m.digestMenuScope == 1 {
		scope1 = activeTagStyle.Render("[•] Individual Channel: " + chDesc)
	}
	content.WriteString("   " + scope1 + "\n")

	scope2 := "[ ] By Topic / Tag(s):"
	if m.digestMenuScope == 2 {
		scope2 = activeTagStyle.Render("[•] By Topic / Tag(s):")
	}
	content.WriteString("   " + scope2 + "\n")
	if m.digestMenuScope == 2 {
		content.WriteString("      " + m.digestTopicInput.View() + "\n")
		trackedTags := m.getTrackedChannelTags()
		if len(trackedTags) > 0 {
			var tagPills []string
			displayLimit := min(8, len(trackedTags))
			for i := 0; i < displayLimit; i++ {
				tagPills = append(tagPills, "#"+trackedTags[i])
			}
			tagsLine := "      Tags: " + strings.Join(tagPills, "  ")
			if len(trackedTags) > displayLimit {
				tagsLine += fmt.Sprintf(" (+%d more)", len(trackedTags)-displayLimit)
			}
			content.WriteString(lipgloss.NewStyle().Foreground(subtleColor).Render(tagsLine) + "\n")
		}
		content.WriteString("      " + lipgloss.NewStyle().Foreground(subtleColor).Render("Tip: combine tags with 'tag1 + tag2' or 'tag1 & tag2'") + "\n")
	}
	content.WriteString("\n")

	// Section 2: Time Period
	now := time.Now()
	content.WriteString(lipgloss.NewStyle().Bold(true).Render("2. Time Period:") + "\n")

	periods := []struct {
		label string
		desc  string
	}{
		{"All Unread Messages", "Only new unread posts since last sync watermark (marks as read)"},
		{"Today (Past 24 Hours)", "Posts published in the last 24 hours (keeps unread markers)"},
		{"Past 7 Days (Week)", "Posts published during the last 7 days"},
		{"Past 30 Days (Month)", "Posts published during the last 30 days"},
		{"Current Month (" + now.Format("January 2006") + ")", "Posts from the 1st of this month to today"},
		{"Previous Month (" + now.AddDate(0, -1, 0).Format("January 2006") + ")", "Full calendar month of posts"},
	}

	for i, p := range periods {
		mark := "[ ]"
		if i == m.digestMenuPeriod {
			mark = "[•]"
		}
		line := fmt.Sprintf("%s %s", mark, p.label)
		desc := fmt.Sprintf("    %s", p.desc)
		if i == m.digestMenuPeriod {
			content.WriteString("   " + activeTagStyle.Render(line) + "\n")
			content.WriteString("   " + lipgloss.NewStyle().Foreground(subtleColor).Render(desc) + "\n")
		} else {
			content.WriteString("   " + line + "\n")
		}
	}

	content.WriteString("\n")
	if m.geminiKey != "" {
		engineStr := fmt.Sprintf("🤖 Active Engine: Google Gemini AI (%s)", m.geminiModel)
		content.WriteString(lipgloss.NewStyle().Foreground(accentColor).Render(engineStr) + "\n")
	} else {
		engineStr := "⚡ Active Engine: Offline Extractive Mode (Rule-based heuristics)"
		tipStr := "   (No Gemini Key configured. Set in [o] Settings for AI neural summarization)"
		content.WriteString(lipgloss.NewStyle().Foreground(highlightColor).Render(engineStr) + "\n")
		content.WriteString(lipgloss.NewStyle().Foreground(subtleColor).Render(tipStr) + "\n")
	}

	content.WriteString("\n" + lipgloss.NewStyle().Foreground(subtleColor).Render("Controls: [Tab/←/→] Toggle Scope  •  [↑/↓] Change Period  •  [Enter] Generate  •  [Esc] Cancel"))

	b.WriteString(borderStyle.Render(content.String()))
	return b.String()
}

func (m *Model) renderHelpBar() string {
	var keys []string

	if m.editingPrompt {
		keys = []string{
			helpKey("[ctrl+s]") + " Save to DB",
			helpKey("[ctrl+r]") + " Reset Default Template",
			helpKey("[Esc]") + " Cancel",
		}
		return statusBarStyle.Width(m.width).Render(strings.Join(keys, "  "))
	}

	if m.readingFullDigest {
		keys = []string{
			helpKey("[↑/↓]") + " Scroll Line",
			helpKey("[PgUp/PgDn/Space]") + " Page",
			helpKey("[Home/End]") + " Top/Bottom",
			helpKey("[Esc/Enter/q]") + " Back to List",
		}
		return statusBarStyle.Width(m.width).Render(strings.Join(keys, "  "))
	}

	if m.showingDigestMenu {
		keys = []string{
			helpKey("[Tab/←/→]") + " Target Scope",
			helpKey("[↑/↓]") + " Select Period",
			helpKey("[Enter]") + " Generate",
			helpKey("[Esc]") + " Cancel",
		}
		return statusBarStyle.Width(m.width).Render(strings.Join(keys, "  "))
	}

	switch m.activeView {
	case ViewChannels:
		if m.showingChannelDetails {
			if m.editingChannelTags {
				keys = []string{
					helpKey("[Enter]") + " Save Tags",
					helpKey("[Esc]") + " Cancel",
				}
			} else {
				keys = []string{
					helpKey("[t]") + " Edit Tags",
					helpKey("[Space]") + " Toggle Sync",
					helpKey("[m]") + " Toggle MCP",
					helpKey("[a]") + " Toggle Ads",
					helpKey("[Esc]") + " Back to Channels",
				}
			}
			return statusBarStyle.Width(m.width).Render(strings.Join(keys, "  "))
		}
		keys = []string{
			helpKey("[Enter]") + " Details & Tags",
			helpKey("[Space]") + " Track",
			helpKey("[m]") + " LLM Access",
			helpKey("[a]") + " Ad Filter",
			helpKey("[g]") + " Digest Menu",
			helpKey("[c]") + " Channel Digest",
			helpKey("[s]") + " Sync",
			helpKey("[/]") + " Search",
			helpKey("[o]") + " Settings",
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
			helpKey("[Enter]") + " Read Full",
			helpKey("[g]") + " Create Digest",
			helpKey("[x]") + " Delete Digest",
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
