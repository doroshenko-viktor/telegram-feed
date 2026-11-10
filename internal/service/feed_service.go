package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"telegram_feed_summarizer/internal/config"
	"telegram_feed_summarizer/internal/domain"
	"telegram_feed_summarizer/internal/llm"
	"telegram_feed_summarizer/internal/pipeline"
	"telegram_feed_summarizer/internal/telegram"
)

// FeedService coordinates Telegram synchronization, ad cleaning, profiling, and digest delivery.
type FeedService struct {
	cfg       *config.Config
	repo      domain.Repository
	tgClient  *telegram.Client
	llmProv   llm.Provider
	botSender *telegram.BotSender
}

// NewFeedService creates an initialized feed service.
func NewFeedService(
	cfg *config.Config,
	repo domain.Repository,
	tgClient *telegram.Client,
	llmProv llm.Provider,
) *FeedService {
	// Check if Telegram Bot credentials are in settings or config
	ctx := context.Background()
	botToken, _ := repo.GetSetting(ctx, "bot_token", "")
	botChatID, _ := repo.GetSetting(ctx, "bot_chat_id", "")

	return &FeedService{
		cfg:       cfg,
		repo:      repo,
		tgClient:  tgClient,
		llmProv:   llmProv,
		botSender: telegram.NewBotSender(botToken, botChatID),
	}
}

// SyncSubscribedChannels fetches all dialogs the user subscribes to and inserts newly discovered channels into the DB.
func (s *FeedService) SyncSubscribedChannels(ctx context.Context) (int, error) {
	if s.tgClient == nil || !s.tgClient.IsConfigured() {
		return 0, fmt.Errorf("telegram client is not configured")
	}

	channels, err := s.tgClient.ListSubscribedChannels(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing subscribed channels: %w", err)
	}

	if err := s.repo.UpsertChannels(ctx, channels); err != nil {
		return 0, fmt.Errorf("saving channels: %w", err)
	}

	return len(channels), nil
}

// SyncTrackedPosts fetches new messages from all channels currently marked with is_tracked = true.
func (s *FeedService) SyncTrackedPosts(ctx context.Context) (int, error) {
	if s.tgClient == nil || !s.tgClient.IsConfigured() {
		return 0, fmt.Errorf("telegram client is not configured")
	}

	trackedChannels, err := s.repo.ListChannels(ctx, true)
	if err != nil {
		return 0, fmt.Errorf("listing tracked channels: %w", err)
	}

	if len(trackedChannels) == 0 {
		// Discover channels first if none tracked
		_, _ = s.SyncSubscribedChannels(ctx)
		return 0, nil
	}

	// Load active dynamic ad patterns from DB
	adPatterns, _ := s.repo.ListAdPatterns(ctx, true)
	cleaner := pipeline.NewCleaner(adPatterns)

	totalNewPosts := 0

	for _, ch := range trackedChannels {
		// Pacing delay to avoid Telegram flood limits
		time.Sleep(150 * time.Millisecond)

		posts, maxMsgID, err := s.tgClient.FetchChannelPosts(ctx, ch, ch.LastSyncMsgID, 100)
		if err != nil {
			fmt.Printf("⚠️ Warning fetching channel %s (%d): %v\n", ch.Title, ch.ID, err)
			continue
		}

		if len(posts) == 0 {
			continue
		}

		// Run through cleaner to classify ads
		var sampleTexts []string
		for _, p := range posts {
			p.IsAd = cleaner.IsAdvertisement(p.Text)
			if len(sampleTexts) < 10 && len(p.Text) > 20 {
				sampleTexts = append(sampleTexts, p.Text)
			}
		}

		// Save posts
		if err := s.repo.UpsertPosts(ctx, posts); err != nil {
			fmt.Printf("⚠️ Warning saving posts for channel %s: %v\n", ch.Title, err)
			continue
		}

		// Update watermark
		_ = s.repo.UpdateWatermark(ctx, ch.ID, maxMsgID)
		totalNewPosts += len(posts)

		// Profile channel if never profiled
		if ch.ProfiledAt == nil && s.llmProv != nil {
			summary, topics, contentType, err := s.llmProv.ProfileChannel(ctx, ch, sampleTexts)
			if err == nil {
				_ = s.repo.UpdateChannelProfile(ctx, ch.ID, summary, topics, contentType)
			}
		}
	}

	return totalNewPosts, nil
}

// GenerateDigest generates an AI digest for the specified topic and delivers it if a bot is configured.
func (s *FeedService) GenerateDigest(ctx context.Context, topic string) (*domain.Digest, error) {
	if s.llmProv == nil || !s.llmProv.HasAI() {
		return nil, fmt.Errorf("local AI is disabled (GEMINI_API_KEY is not set). Digests can be queried on demand via MCP in your LLM chat")
	}

	if topic == "" {
		topic = "all"
	}

	// Fetch unread posts, filtering out ads
	posts, err := s.repo.GetPosts(ctx, domain.PostFilter{
		Topic:      topic,
		FilterAds:  true,
		UnreadOnly: true,
		Limit:      100,
	})
	if err != nil {
		return nil, fmt.Errorf("fetching posts for digest: %w", err)
	}

	if len(posts) == 0 {
		return nil, fmt.Errorf("no unread posts found for topic '%s'", topic)
	}

	digest, err := s.llmProv.GenerateDigest(ctx, topic, posts)
	if err != nil {
		return nil, fmt.Errorf("generating digest with LLM: %w", err)
	}

	// Persist digest into database
	if err := s.repo.SaveDigest(ctx, digest); err != nil {
		return nil, fmt.Errorf("saving digest: %w", err)
	}

	// Optional: Deliver via dedicated Telegram Bot if enabled
	botEnabled, _ := s.repo.GetSetting(ctx, "delivery_bot_enabled", "false")
	if botEnabled == "true" {
		token, _ := s.repo.GetSetting(ctx, "bot_token", s.cfg.Telegram.BotToken)
		chatID, _ := s.repo.GetSetting(ctx, "bot_chat_id", s.cfg.Telegram.BotChatID)
		botSender := telegram.NewBotSender(token, chatID)
		if botSender.IsConfigured() {
			if err := botSender.SendDigest(ctx, digest.SummaryMarkdown); err != nil {
				fmt.Printf("⚠️ Warning sending digest to Telegram bot: %v\n", err)
			}
		}
	}

	// Save to local Markdown file
	if s.cfg.Daemon.OutputDir != "" {
		_ = os.MkdirAll(s.cfg.Daemon.OutputDir, 0755)
		filename := fmt.Sprintf("digest-%s-%s.md", topic, time.Now().Format("2006-01-02-150405"))
		fullPath := filepath.Join(s.cfg.Daemon.OutputDir, filename)
		_ = os.WriteFile(fullPath, []byte(digest.SummaryMarkdown), 0644)
	}

	// Mark processed posts as read
	var postIDs []int64
	for _, p := range posts {
		postIDs = append(postIDs, p.ID)
	}
	_ = s.repo.MarkPostsRead(ctx, postIDs)

	return digest, nil
}
