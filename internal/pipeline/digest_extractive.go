package pipeline

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"telegram_feed_summarizer/internal/domain"
)

var (
	urlRegex            = regexp.MustCompile(`https?://[^\s<>"'()[\]{}|\\^` + "`" + `]+`)
	markdownLinkRegex   = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)
	githubRepoRegex     = regexp.MustCompile(`(?i)^https?://github\.com/([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+)`)
	telegramInviteRegex = regexp.MustCompile(`(?i)^https?://t\.me/(\+|joinchat/)`)
	telegramChannelOnly = regexp.MustCompile(`(?i)^https?://t\.me/([a-zA-Z0-9_]+)/?$`)
)

// ExtractedLink represents an external resource (GitHub repository, documentation, or technical article)
// preserved from a post.
type ExtractedLink struct {
	Label    string
	URL      string
	IsGitHub bool
}

// GenerateExtractiveDigest builds a structured markdown digest directly from unread posts
// without requiring an external LLM API key.
func GenerateExtractiveDigest(topic string, posts []*domain.Post, channels []*domain.Channel) (*domain.Digest, error) {
	return GenerateExtractiveDigestWithTitle("", topic, posts, channels)
}

// GenerateExtractiveDigestWithTitle builds a structured markdown digest with an optional custom title.
func GenerateExtractiveDigestWithTitle(customTitle string, topic string, posts []*domain.Post, channels []*domain.Channel) (*domain.Digest, error) {
	if len(posts) == 0 {
		return nil, fmt.Errorf("no posts available to summarize")
	}

	channelMap := make(map[int64]*domain.Channel)
	for _, ch := range channels {
		channelMap[ch.ID] = ch
	}

	// Filter out non-substantive or noise posts
	var validPosts []*domain.Post
	for _, p := range posts {
		if p.IsAd {
			continue
		}
		trimmed := strings.TrimSpace(p.Text)
		if countAlphanumeric(trimmed) < 10 {
			continue
		}
		validPosts = append(validPosts, p)
	}

	if len(validPosts) == 0 {
		return nil, fmt.Errorf("no substantive posts available after filtering noise")
	}

	// Group posts by channel ID
	grouped := make(map[int64][]*domain.Post)
	earliest := validPosts[0].Date
	latest := validPosts[0].Date

	for _, p := range validPosts {
		if p.Date.Before(earliest) {
			earliest = p.Date
		}
		if p.Date.After(latest) {
			latest = p.Date
		}
		grouped[p.ChannelID] = append(grouped[p.ChannelID], p)
	}

	// Collect unique channel IDs sorted by activity
	type chStat struct {
		id    int64
		count int
		title string
	}
	var stats []chStat
	for id, chPosts := range grouped {
		title := fmt.Sprintf("Channel #%d", id)
		if ch, ok := channelMap[id]; ok && ch.Title != "" {
			title = ch.Title
		}
		stats = append(stats, chStat{
			id:    id,
			count: len(chPosts),
			title: title,
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].count > stats[j].count
	})

	var sb strings.Builder
	topicDisplay := strings.ToUpper(topic)
	if customTitle != "" {
		topicDisplay = customTitle
	} else if topic == "" || topic == "all" {
		if len(stats) == 1 {
			topicDisplay = stats[0].title
		} else {
			topicDisplay = "ALL TRACKED CHANNELS"
		}
	}

	sb.WriteString(fmt.Sprintf("# 📰 Telegram Digest: %s\n\n", topicDisplay))
	sb.WriteString(fmt.Sprintf("**Period:** %s — %s\n",
		earliest.Format("Jan 02, 15:04"),
		latest.Format("Jan 02, 15:04"),
	))
	sb.WriteString(fmt.Sprintf("**Coverage:** %d posts across %d channel(s) *(Extractive Mode)*\n\n",
		len(validPosts), len(stats),
	))
	sb.WriteString("---\n\n")

	var channelIDs []int64
	for _, s := range stats {
		channelIDs = append(channelIDs, s.id)
		chPosts := grouped[s.id]
		ch := channelMap[s.id]

		channelTitle := s.title
		chURL := buildChannelURL(ch)
		var channelHeader string
		if chURL != "" {
			channelHeader = fmt.Sprintf("### 📢 [%s](%s)", channelTitle, chURL)
		} else {
			channelHeader = fmt.Sprintf("### 📢 %s", channelTitle)
		}
		sb.WriteString(channelHeader + "\n\n")

		// Separate relevant substantive posts from low-value noise/gossip
		var relevantPosts []*domain.Post
		var filteredNoiseCount int
		for _, p := range chPosts {
			if calculatePostSubstanceScore(p) < 0 {
				filteredNoiseCount++
			} else {
				relevantPosts = append(relevantPosts, p)
			}
		}

		// Sort relevant posts by substance & high-signal technical value first
		sort.Slice(relevantPosts, func(i, j int) bool {
			scoreI := calculatePostSubstanceScore(relevantPosts[i])
			scoreJ := calculatePostSubstanceScore(relevantPosts[j])
			if scoreI != scoreJ {
				return scoreI > scoreJ
			}
			return relevantPosts[i].Date.After(relevantPosts[j].Date)
		})

		// Include top posts (up to 10 per channel)
		displayLimit := 10
		countIncluded := 0

		for _, p := range relevantPosts {
			if countIncluded >= displayLimit {
				break
			}

			postURL := buildPostURL(ch, p.MsgID)

			headline, body, bullets, links := extractPostSummary(p.Text)
			if headline == "" && body == "" && len(bullets) == 0 {
				continue
			}

			entry := formatPostEntry(headline, body, bullets, links, postURL)
			sb.WriteString(entry)
			countIncluded++
		}

		remainingRelevant := len(relevantPosts) - countIncluded
		if remainingRelevant > 0 {
			sb.WriteString(fmt.Sprintf("*... and %d more substantive posts in this channel*\n\n", remainingRelevant))
		}
		if filteredNoiseCount > 0 {
			sb.WriteString(fmt.Sprintf("*(%d noise/fluff posts filtered out by relevance criteria)*\n\n", filteredNoiseCount))
		}
	}

	digestID := fmt.Sprintf("digest-%s-%d", topic, time.Now().Unix())
	displayTitle := fmt.Sprintf("%s Digest (Extractive) - %s", topicDisplay, time.Now().Format("Jan 02, 2006"))
	if customTitle != "" {
		displayTitle = fmt.Sprintf("%s (Extractive) - %s", customTitle, time.Now().Format("Jan 02, 2006"))
	}
	return &domain.Digest{
		ID:              digestID,
		Title:           displayTitle,
		Topic:           topic,
		PeriodStart:     earliest,
		PeriodEnd:       latest,
		ChannelIDs:      channelIDs,
		SummaryMarkdown: sb.String(),
		PostCount:       len(validPosts),
		CreatedAt:       time.Now(),
	}, nil
}

// formatPostEntry renders a single post summary item in clean Markdown.
// Crucially: no engagement metrics (views/forwards) or timestamp noise.
func formatPostEntry(headline string, body string, bullets []string, links []ExtractedLink, postURL string) string {
	var sb strings.Builder

	if headline != "" {
		sb.WriteString(fmt.Sprintf("- **%s**\n", headline))
	} else if body != "" {
		sb.WriteString(fmt.Sprintf("- %s\n", body))
		body = "" // already output as bullet lead
	}

	if body != "" {
		sb.WriteString(fmt.Sprintf("  %s\n", body))
	}

	for _, b := range bullets {
		sb.WriteString(fmt.Sprintf("  • %s\n", b))
	}

	// Format resource links and original post reference
	var linkItems []string
	for _, l := range links {
		linkItems = append(linkItems, fmt.Sprintf("[%s](%s)", l.Label, l.URL))
	}

	if postURL != "" {
		linkItems = append(linkItems, fmt.Sprintf("[Telegram Post ↗](%s)", postURL))
	}

	if len(linkItems) > 0 {
		sb.WriteString(fmt.Sprintf("  🔗 %s\n", strings.Join(linkItems, " • ")))
	}

	sb.WriteString("\n")
	return sb.String()
}

// extractPostSummary parses a Telegram post into headline, body, bullet items, and preserved external links.
func extractPostSummary(text string) (headline string, body string, bullets []string, links []ExtractedLink) {
	links = extractLinks(text)
	cleanedLines := cleanPostLines(text)
	if len(cleanedLines) == 0 {
		return "", "", nil, links
	}

	// Separate bullet list items from prose lines
	var proseLines []string
	for _, line := range cleanedLines {
		if isBulletLine(line) {
			if len(bullets) < 4 { // keep up to 4 key highlights
				bullets = append(bullets, cleanBulletLine(line))
			}
		} else {
			proseLines = append(proseLines, line)
		}
	}

	if len(proseLines) == 0 && len(bullets) > 0 {
		headline = bullets[0]
		bullets = bullets[1:]
		return headline, "", bullets, links
	}

	if len(proseLines) == 0 {
		return "", "", bullets, links
	}

	firstLine := proseLines[0]
	rFirstLine := []rune(firstLine)

	// In Telegram channels, the first line is typically a concise headline (<= 140 runes).
	// If so, treat proseLines[0] as the headline and remaining proseLines[1:] as the body.
	if len(rFirstLine) <= 140 || strings.HasSuffix(firstLine, ":") {
		headline = cleanHeadline(firstLine)
		if len(proseLines) > 1 {
			remainingProse := strings.Join(proseLines[1:], " ")
			sentences := splitSentences(remainingProse)
			var bodyParts []string
			bodyRunes := 0
			for _, s := range sentences {
				sRunes := len([]rune(s))
				if bodyRunes+sRunes > 380 && len(bodyParts) > 0 {
					break
				}
				bodyParts = append(bodyParts, s)
				bodyRunes += sRunes
				if len(bodyParts) >= 3 {
					break
				}
			}
			body = strings.Join(bodyParts, " ")
		}
	} else {
		// Single long paragraph where first line is itself > 140 runes. Split by sentence.
		joinedProse := strings.Join(proseLines, " ")
		sentences := splitSentences(joinedProse)
		if len(sentences) > 0 {
			headline = cleanHeadline(sentences[0])
			if len(sentences) > 1 {
				var bodyParts []string
				bodyRunes := 0
				for _, s := range sentences[1:] {
					sRunes := len([]rune(s))
					if bodyRunes+sRunes > 380 && len(bodyParts) > 0 {
						break
					}
					bodyParts = append(bodyParts, s)
					bodyRunes += sRunes
					if len(bodyParts) >= 3 {
						break
					}
				}
				body = strings.Join(bodyParts, " ")
			}
		}
	}

	return headline, body, bullets, links
}

var genericLinkLabels = map[string]bool{
	"здесь": true, "тут": true, "клик": true, "ссылке": true, "ссылка": true,
	"here": true, "click": true, "link": true, "source": true, "подробнее": true,
	"источник": true, "читать": true, "сайт": true, "подробнее по ссылке": true,
	"перейти": true, "по ссылке": true,
}

// extractLinks identifies external resources, GitHub repositories, and documentation URLs.
func extractLinks(text string) []ExtractedLink {
	var results []ExtractedLink
	seen := make(map[string]bool)

	// 1. Extract markdown links [label](url)
	mdMatches := markdownLinkRegex.FindAllStringSubmatch(text, -1)
	for _, m := range mdMatches {
		rawURL := cleanURL(m[2])
		if rawURL == "" || isIgnoredTelegramURL(rawURL) || seen[rawURL] {
			continue
		}
		seen[rawURL] = true

		label := strings.TrimSpace(m[1])
		isGH, ghLabel := parseGitHubRepo(rawURL)
		if isGH {
			results = append(results, ExtractedLink{Label: ghLabel, URL: rawURL, IsGitHub: true})
		} else {
			if label == "" || strings.HasPrefix(label, "http") || genericLinkLabels[strings.ToLower(label)] {
				label = cleanURLLabel(rawURL)
			}
			results = append(results, ExtractedLink{Label: label, URL: rawURL, IsGitHub: false})
		}
	}

	// 2. Extract raw URLs
	rawMatches := urlRegex.FindAllString(text, -1)
	for _, raw := range rawMatches {
		u := cleanURL(raw)
		if u == "" || isIgnoredTelegramURL(u) || seen[u] {
			continue
		}
		seen[u] = true

		isGH, ghLabel := parseGitHubRepo(u)
		if isGH {
			results = append(results, ExtractedLink{Label: ghLabel, URL: u, IsGitHub: true})
		} else {
			results = append(results, ExtractedLink{Label: cleanURLLabel(u), URL: u, IsGitHub: false})
		}
	}

	// Sort GitHub links first
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].IsGitHub != results[j].IsGitHub {
			return results[i].IsGitHub
		}
		return false
	})

	// Limit to top 5 external links per post
	if len(results) > 5 {
		results = results[:5]
	}

	return results
}

func parseGitHubRepo(rawURL string) (bool, string) {
	m := githubRepoRegex.FindStringSubmatch(rawURL)
	if len(m) >= 3 {
		owner := m[1]
		repo := strings.TrimSuffix(m[2], ".git")
		return true, fmt.Sprintf("GitHub: %s/%s", owner, repo)
	}
	return false, ""
}

func isIgnoredTelegramURL(rawURL string) bool {
	if telegramInviteRegex.MatchString(rawURL) {
		return true
	}
	if telegramChannelOnly.MatchString(rawURL) {
		return true
	}
	return false
}

func cleanURL(raw string) string {
	u := strings.TrimRight(raw, ".,;:!?)]\"'}>")
	return strings.TrimSpace(u)
}

func cleanURLLabel(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return rawURL
	}
	host := strings.TrimPrefix(parsed.Host, "www.")
	path := strings.TrimRight(parsed.Path, "/")
	if path != "" && path != "/" {
		if len(path) > 25 {
			parts := strings.Split(path, "/")
			if len(parts) > 1 {
				path = "/" + parts[len(parts)-1]
			}
		}
		return host + path
	}
	return host
}

func cleanChannelIDForURL(id int64) string {
	s := fmt.Sprintf("%d", id)
	s = strings.TrimPrefix(s, "-100")
	s = strings.TrimPrefix(s, "-")
	return s
}

func buildChannelURL(ch *domain.Channel) string {
	if ch == nil {
		return ""
	}
	if ch.Username != "" {
		return fmt.Sprintf("https://t.me/%s", ch.Username)
	}
	if ch.ID != 0 {
		cidStr := cleanChannelIDForURL(ch.ID)
		if cidStr != "" {
			return fmt.Sprintf("https://t.me/c/%s", cidStr)
		}
	}
	return ""
}

func buildPostURL(ch *domain.Channel, msgID int) string {
	if ch == nil || msgID <= 0 {
		return ""
	}
	if ch.Username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", ch.Username, msgID)
	}
	if ch.ID != 0 {
		cidStr := cleanChannelIDForURL(ch.ID)
		if cidStr != "" {
			return fmt.Sprintf("https://t.me/c/%s/%d", cidStr, msgID)
		}
	}
	return ""
}

var promoKeywords = []string{
	"подписывайтесь",
	"подпишись",
	"подписаться",
	"канал автора",
	"наш канал",
	"наш чат",
	"наш телеграм",
	"ссылка в закреп",
	"читать далее",
	"продолжение в",
	"источник:",
	"источник :",
	"ставьте лайк",
	"ставьте реакци",
	"жми ❤️",
	"жми 🔥",
	"жми лайк",
	"subscribe to",
	"subscribe:",
	"follow us",
	"join our channel",
	"join us on telegram",
	"read more at",
	"source:",
	"leave a reaction",
	"hit like",
}

var watermarkSuffixRegex = regexp.MustCompile(`(?i)\s*(?:[|—–-]\s*|❤️|🔥|👍)\s*(?:pda|4pda|habr|хабр|tproger|тпрогер|tg|chat|channel|@[\w_]+)\s*$`)
var pureWatermarkRegex = regexp.MustCompile(`(?i)^(?:[|—–-]\s*|❤️|🔥|👍)?\s*(?:pda|4pda|habr|хабр|tproger|тпрогер|tg|chat|channel|@[\w_]+)\s*$`)

var gossipAndFluffKeywords = []string{
	// Gossip & relationship drama
	"изменил жене",
	"изменила мужу",
	"скандал вокруг главы",
	"любовниц",
	"развод ",
	"расстались",
	"измена",
	// Gaming releases, console jailbreaks, preloads, and game-specific adaptations
	"предзагрузк",
	"preload",
	"скачать в psn",
	"новозеландский регион",
	"во сколько скачать",
	"джейлбрейк",
	"jailbreak",
	"gta 6",
	"gta vi",
	"гта 6",
	"гта vi",
	"ps5",
	"ps4",
	"xbox series",
	"death stranding",
	"коджим",
	"kojima",
	"аниме-адаптац",
	"dark heresy",
	"warhammer 40,000",
	"warhammer 40k",
	"baldur's gate теперь",
	// Memes and trivial cosmetic rumors
	"дрозофилу заставили",
	"муху заставили",
	"муху превратить",
	"шутка дня",
	"сердечки вместо кружочков",
	"вместо кружочков",
}

func isGossipOrLowValueFluff(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range gossipAndFluffKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func calculatePostSubstanceScore(p *domain.Post) int {
	text := p.Text
	lower := strings.ToLower(text)

	// Heavy penalty for gossip, scandals, preload schedules, memes
	if isGossipOrLowValueFluff(lower) {
		return -1000
	}

	score := 0

	// GitHub repo presence is the highest signal of actionable tech (+500)
	if strings.Contains(lower, "github.com/") {
		score += 500
	}

	// External URLs / documentation links (+250)
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") {
		score += 250
	}

	// Technical release / breakthrough keywords (+50 each)
	techKeywords := []string{
		"релиз", "release", "версия", "выпустил", "анонсировал",
		"библиотек", "инструмент", "фреймворк", "учебник",
		"архитектур", "протокол", "benchmark", "алгоритм",
		"нейросеть", "модель", "стартап", "открытый исходный",
	}
	for _, kw := range techKeywords {
		if strings.Contains(lower, kw) {
			score += 50
		}
	}

	// If message has bullet points / features (+50)
	if strings.Contains(text, "\n- ") || strings.Contains(text, "\n• ") || strings.Contains(text, "\n* ") {
		score += 50
	}

	// Add normalized engagement (views/1000 + forwards*2)
	score += (p.Views / 1000) + (p.Forwards * 2)

	return score
}

func cleanPostLines(text string) []string {
	lines := strings.Split(text, "\n")
	var cleaned []string

	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}

		if isHashtagLine(trimmed) {
			continue
		}

		// Strip standalone channel signatures / watermarks
		if pureWatermarkRegex.MatchString(trimmed) {
			continue
		}

		// Strip trailing watermark from line
		trimmed = watermarkSuffixRegex.ReplaceAllString(trimmed, "")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)
		isPromo := false
		for _, kw := range promoKeywords {
			if strings.Contains(lower, kw) {
				isPromo = true
				break
			}
		}
		if isPromo {
			continue
		}

		cleaned = append(cleaned, trimmed)
	}

	return cleaned
}

func isHashtagLine(line string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if !strings.HasPrefix(f, "#") {
			return false
		}
	}
	return true
}

func isBulletLine(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "• ") {
		return true
	}
	// Check for numbered items e.g. "1. ", "2) "
	if len(line) >= 3 && unicode.IsDigit(rune(line[0])) {
		if line[1] == '.' || line[1] == ')' {
			return true
		}
		if unicode.IsDigit(rune(line[1])) && len(line) >= 4 && (line[2] == '.' || line[2] == ')') {
			return true
		}
	}
	return false
}

func cleanBulletLine(line string) string {
	line = strings.TrimPrefix(line, "- ")
	line = strings.TrimPrefix(line, "* ")
	line = strings.TrimPrefix(line, "• ")
	if len(line) >= 3 && unicode.IsDigit(rune(line[0])) {
		if line[1] == '.' || line[1] == ')' {
			line = strings.TrimSpace(line[2:])
		} else if unicode.IsDigit(rune(line[1])) && len(line) >= 4 && (line[2] == '.' || line[2] == ')') {
			line = strings.TrimSpace(line[3:])
		}
	}
	return strings.TrimSpace(line)
}

func cleanHeadline(s string) string {
	s = strings.TrimSpace(s)
	// Strip leading dashes or asterisks
	s = strings.TrimPrefix(s, "- ")
	s = strings.TrimPrefix(s, "* ")
	s = strings.TrimPrefix(s, "• ")
	// If wrapped in bold **Headline**, unwrap so formatPostEntry can wrap consistently
	if strings.HasPrefix(s, "**") && strings.HasSuffix(s, "**") && len(s) > 4 {
		s = s[2 : len(s)-2]
	}
	// Strip trailing colon or exclamation if excessively repeated
	s = strings.TrimRight(s, ": \t")
	return strings.TrimSpace(s)
}

func countAlphanumeric(s string) int {
	cnt := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cnt++
		}
	}
	return cnt
}

// splitSentences divides text into complete sentences based on punctuation boundaries (. ! ? …).
// Crucially respects decimal numbers (1.25) and domain dots (github.com).
func splitSentences(text string) []string {
	var sentences []string
	runes := []rune(text)
	n := len(runes)
	start := 0

	for i := 0; i < n; i++ {
		r := runes[i]
		isTerminator := r == '.' || r == '!' || r == '?' || r == '…'
		if isTerminator {
			// Avoid splitting decimal numbers (1.25) or domain names (example.com)
			if r == '.' && i > 0 && i+1 < n {
				prev := runes[i-1]
				next := runes[i+1]
				if unicode.IsDigit(prev) && unicode.IsDigit(next) {
					continue
				}
				if unicode.IsLetter(prev) && unicode.IsLetter(next) {
					continue
				}
			}

			// Advance past consecutive terminators (e.g. "...", "!!", "?!")
			for i+1 < n && (runes[i+1] == '.' || runes[i+1] == '!' || runes[i+1] == '?' || runes[i+1] == '…') {
				i++
			}

			// Check if at end of text or followed by whitespace or quote/bracket
			if i+1 == n || unicode.IsSpace(runes[i+1]) || runes[i+1] == '"' || runes[i+1] == '»' || runes[i+1] == ')' {
				sentence := strings.TrimSpace(string(runes[start : i+1]))
				if len(sentence) > 0 {
					sentences = append(sentences, sentence)
				}
				start = i + 1
			}
		}
	}

	if start < n {
		tail := strings.TrimSpace(string(runes[start:]))
		if len(tail) > 0 {
			sentences = append(sentences, tail)
		}
	}
	return sentences
}
