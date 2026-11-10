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
		model = "gemini-3.8-flash"
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

// GenerateDigest generates a structured markdown digest from a slice of posts.
func (g *GeminiProvider) GenerateDigest(ctx context.Context, topic string, posts []*domain.Post) (*domain.Digest, error) {
	if len(posts) == 0 {
		return nil, fmt.Errorf("no posts available to summarize")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are an expert technical digest editor. Create a concise, high-signal Telegram digest about [%s] from the following raw posts.\n", topic))
	sb.WriteString("Requirements:\n")
	sb.WriteString("1. Group into: 🔥 Executive Summary, 🚀 Releases & Tools, 💡 Deep Dives & Articles, 📌 Quick Notes.\n")
	sb.WriteString("2. Omit any advertisements, spam, course sales, or low-value promos.\n")
	sb.WriteString("3. Format in clean, readable Markdown.\n\n")
	sb.WriteString("Raw Channel Posts (safely delimited):\n")

	earliest := posts[0].Date
	latest := posts[0].Date
	channelIDMap := make(map[int64]bool)

	for _, p := range posts {
		if p.Date.Before(earliest) {
			earliest = p.Date
		}
		if p.Date.After(latest) {
			latest = p.Date
		}
		channelIDMap[p.ChannelID] = true

		sb.WriteString(fmt.Sprintf("<post id=\"%d\" channel=\"%d\" date=\"%s\">\n", p.MsgID, p.ChannelID, p.Date.Format(time.RFC3339)))
		sb.WriteString(p.Text)
		sb.WriteString("\n</post>\n\n")
	}

	result, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(sb.String()), nil)
	if err != nil {
		return nil, fmt.Errorf("generating digest content with %s: %w", g.model, err)
	}

	summaryText := ""
	if result != nil {
		summaryText = result.Text()
	}
	if summaryText == "" {
		return nil, fmt.Errorf("gemini returned an empty response")
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
