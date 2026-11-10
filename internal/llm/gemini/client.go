package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"telegram_feed_summarizer/internal/config"
	"telegram_feed_summarizer/internal/domain"
	"telegram_feed_summarizer/internal/logger"
	"telegram_feed_summarizer/internal/pipeline"
)

// GeminiProvider implements llm.Provider using Google Gemini via google.golang.org/genai.
type GeminiProvider struct {
	client *genai.Client
	model  string
}

// ValidateAPIKey verifies that the provided Gemini API key is valid by querying the Gemini API.
func ValidateAPIKey(ctx context.Context, apiKey string) error {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  trimmed,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return fmt.Errorf("initializing client: %w", err)
	}

	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err = client.Models.List(checkCtx, nil)
	if err != nil {
		return fmt.Errorf("invalid API key: %w", err)
	}

	return nil
}

// NewGeminiProvider creates a new Gemini provider.
func NewGeminiProvider(ctx context.Context, cfg config.GeminiConfig) (*GeminiProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini API key is required")
	}

	model := cfg.Model
	if model == "" {
		model = "gemini-3.5-flash"
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("creating gemini client: %w", err)
	}

	return &GeminiProvider{
		client: client,
		model:  model,
	}, nil
}

func (g *GeminiProvider) HasAI() bool {
	return true
}

// GenerateDigest generates a structured markdown digest from a slice of posts using the provided prompt template.
func (g *GeminiProvider) GenerateDigest(ctx context.Context, topic string, posts []*domain.Post, channels []*domain.Channel, promptTemplate string) (*domain.Digest, error) {
	if len(posts) == 0 {
		return nil, fmt.Errorf("no posts available to summarize")
	}

	channelMap := make(map[int64]*domain.Channel)
	for _, ch := range channels {
		channelMap[ch.ID] = ch
	}

	prompt := strings.TrimSpace(promptTemplate)
	if prompt == "" {
		prompt = domain.DefaultDigestPrompt
	}

	// Interpolate topic placeholder if present
	if strings.Contains(prompt, "{{topic}}") {
		prompt = strings.ReplaceAll(prompt, "{{topic}}", topic)
	} else if strings.Contains(prompt, "%s") {
		prompt = fmt.Sprintf(prompt, topic)
	} else {
		prompt = fmt.Sprintf("Topic: %s\n\n%s", topic, prompt)
	}

	var sb strings.Builder
	sb.WriteString(prompt)
	sb.WriteString("\n\nRaw Channel Posts (grouped by source channel):\n")

	earliest := posts[0].Date
	latest := posts[0].Date
	channelIDMap := make(map[int64]bool)

	// Group posts by source channel
	grouped := make(map[int64][]*domain.Post)
	var orderedChannelIDs []int64
	for _, p := range posts {
		if p.Date.Before(earliest) {
			earliest = p.Date
		}
		if p.Date.After(latest) {
			latest = p.Date
		}
		channelIDMap[p.ChannelID] = true

		if _, exists := grouped[p.ChannelID]; !exists {
			orderedChannelIDs = append(orderedChannelIDs, p.ChannelID)
		}
		grouped[p.ChannelID] = append(grouped[p.ChannelID], p)
	}

	for _, chID := range orderedChannelIDs {
		chPosts := grouped[chID]
		ch := channelMap[chID]
		chTitle := fmt.Sprintf("Channel %d", chID)
		var chUsername, chURL string
		if ch != nil {
			chTitle = ch.Title
			chUsername = ch.Username
			if ch.Username != "" {
				chURL = fmt.Sprintf("https://t.me/%s", ch.Username)
			} else {
				chURL = fmt.Sprintf("https://t.me/c/%d", ch.ID)
			}
		}

		sb.WriteString(fmt.Sprintf("<channel id=\"%d\" title=\"%s\" username=\"%s\" url=\"%s\">\n", chID, chTitle, chUsername, chURL))
		for _, p := range chPosts {
			var postURL string
			if ch != nil && ch.Username != "" {
				postURL = fmt.Sprintf("https://t.me/%s/%d", ch.Username, p.MsgID)
			} else if ch != nil {
				postURL = fmt.Sprintf("https://t.me/c/%d/%d", ch.ID, p.MsgID)
			}
			sb.WriteString(fmt.Sprintf("  <post id=\"%d\" url=\"%s\" date=\"%s\">\n", p.MsgID, postURL, p.Date.Format(time.RFC3339)))
			sb.WriteString("    " + strings.ReplaceAll(strings.TrimSpace(p.Text), "\n", "\n    ") + "\n")
			sb.WriteString("  </post>\n")
		}
		sb.WriteString("</channel>\n\n")
	}

	modelsToTry := []string{g.model}
	if g.model != "gemini-3.5-flash" {
		modelsToTry = append(modelsToTry, "gemini-3.5-flash")
	}
	if g.model != "gemini-3.5-flash-lite" {
		modelsToTry = append(modelsToTry, "gemini-3.5-flash-lite")
	}

	var result *genai.GenerateContentResponse
	var lastErr error

	for _, tryModel := range modelsToTry {
		logger.Infof("Calling Gemini API with model '%s' for topic '%s' (%d posts)...", tryModel, topic, len(posts))
		res, err := g.client.Models.GenerateContent(ctx, tryModel, genai.Text(sb.String()), nil)
		if err == nil && res != nil && res.Text() != "" {
			result = res
			logger.Infof("Gemini model '%s' successfully synthesized digest (%d characters)", tryModel, len(res.Text()))
			break
		}
		lastErr = err
		logger.Warnf("Gemini model '%s' failed: %v", tryModel, err)
	}

	if result == nil || result.Text() == "" {
		logger.Errorf("All Gemini models failed: %v", lastErr)
		return nil, fmt.Errorf("generating digest with gemini: %w", lastErr)
	}

	summaryText := result.Text()

	// Guarantee that single channel summaries explicitly start with their channel header
	if len(orderedChannelIDs) == 1 {
		singleCh := channelMap[orderedChannelIDs[0]]
		if singleCh != nil && !strings.Contains(summaryText, singleCh.Title) {
			var chHeader string
			if singleCh.Username != "" {
				chHeader = fmt.Sprintf("### 📢 [%s](https://t.me/%s) (@%s)", singleCh.Title, singleCh.Username, singleCh.Username)
			} else {
				chHeader = fmt.Sprintf("### 📢 [%s](https://t.me/c/%d)", singleCh.Title, singleCh.ID)
			}
			summaryText = chHeader + "\n\n" + summaryText
		}
	}

	var channelIDs []int64
	for id := range channelIDMap {
		channelIDs = append(channelIDs, id)
	}

	digestID := fmt.Sprintf("digest-%s-%d", topic, time.Now().Unix())
	digest := &domain.Digest{
		ID:              digestID,
		Title:           fmt.Sprintf("%s Digest - %s", strings.ToUpper(topic), time.Now().Format("Jan 02, 2006")),
		Topic:           topic,
		PeriodStart:     earliest,
		PeriodEnd:       latest,
		ChannelIDs:      channelIDs,
		SummaryMarkdown: summaryText,
		PostCount:       len(posts),
		CreatedAt:       time.Now(),
	}

	return digest, nil
}

// ProfileChannel extracts semantic topics, summary, and content type from channel metadata.
func (g *GeminiProvider) ProfileChannel(ctx context.Context, ch *domain.Channel, samplePosts []string) (string, []string, string, error) {
	prompt := fmt.Sprintf(`Analyze this Telegram channel and output a strict JSON object:
{
  "summary": "1-2 sentence description of what this channel focuses on",
  "topics": ["standardized", "lowercase", "tags", "like", "go", "kubernetes"],
  "content_type": "news | deep_dive | tutorial | release | general"
}

Channel Title: %s
Channel Bio: %s
Sample Recent Posts:
%s

Output ONLY valid JSON.`, ch.Title, ch.About, strings.Join(samplePosts, "\n---\n"))

	result, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(prompt), nil)
	if err != nil {
		// Fallback to heuristic if API fails or quota exceeded
		s, t, c := pipeline.HeuristicProfileChannel(ch, samplePosts)
		return s, t, c, nil
	}

	raw := result.Text()
	// Strip markdown code fence if present
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var parsed struct {
		Summary     string   `json:"summary"`
		Topics      []string `json:"topics"`
		ContentType string   `json:"content_type"`
	}

	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		s, t, c := pipeline.HeuristicProfileChannel(ch, samplePosts)
		return s, t, c, nil
	}

	if parsed.Summary == "" {
		parsed.Summary = ch.About
	}
	if len(parsed.Topics) == 0 {
		parsed.Topics = []string{"general"}
	}
	if parsed.ContentType == "" {
		parsed.ContentType = "general"
	}

	return parsed.Summary, parsed.Topics, parsed.ContentType, nil
}
