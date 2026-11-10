package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"telegram_feed_summarizer/internal/config"
	"telegram_feed_summarizer/internal/domain"
	"telegram_feed_summarizer/internal/llm"
	"telegram_feed_summarizer/internal/llm/gemini"
	"telegram_feed_summarizer/internal/logger"
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

		posts, maxMsgID, err := s.tgClient.FetchChannelPosts(ctx, ch, ch.LastSyncMsgID, 1000)
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

// DigestTimeRange specifies the temporal scope for digest aggregation.
type DigestTimeRange string

const (
	TimeRangeUnread       DigestTimeRange = "unread"
	TimeRangeToday        DigestTimeRange = "today"
	TimeRangePast7Days    DigestTimeRange = "week"
	TimeRangePast30Days   DigestTimeRange = "month"
	TimeRangeCurrentMonth DigestTimeRange = "current_month"
	TimeRangePrevMonth    DigestTimeRange = "prev_month"
)

// DigestOptions specifies channel scope and temporal parameters for creating a digest.
type DigestOptions struct {
	ChannelID int64           // 0 for all tracked channels, or specific channel ID
	Topic     string          // Optional topic tag (default "all")
	TimeRange DigestTimeRange // "unread", "today", "week", "month", "current_month", "prev_month"
	MaxPosts  int             // Cap on posts to analyze (default 500)
}

// GenerateDigest generates an AI or extractive digest for the specified topic and delivers it if a bot is configured.
func (s *FeedService) GenerateDigest(ctx context.Context, topic string) (*domain.Digest, error) {
	return s.GenerateDigestWithOptions(ctx, DigestOptions{
		Topic:     topic,
		TimeRange: TimeRangeUnread,
	})
}

// GenerateDigestWithOptions creates a digest for an individual channel or all channels over a chosen time period.
func (s *FeedService) GenerateDigestWithOptions(ctx context.Context, opts DigestOptions) (*domain.Digest, error) {
	now := time.Now()
	if opts.TimeRange == "" {
		opts.TimeRange = TimeRangeUnread
	}
	if opts.MaxPosts <= 0 {
		opts.MaxPosts = 500
	}

	filter := domain.PostFilter{
		FilterAds: true,
		Limit:     opts.MaxPosts,
	}

	var (
		targetName    string
		topic         string
		targetChannel *domain.Channel
	)

	if opts.ChannelID > 0 {
		ch, err := s.repo.GetChannel(ctx, opts.ChannelID)
		if err != nil {
			return nil, fmt.Errorf("getting channel %d: %w", opts.ChannelID, err)
		}
		targetChannel = ch
		targetName = ch.Title
		topic = ch.Title
		filter.ChannelIDs = []int64{opts.ChannelID}
	} else if opts.Topic != "" && opts.Topic != "all" {
		targetName = fmt.Sprintf("Topic: %s", opts.Topic)
		topic = opts.Topic
		filter.TrackedOnly = true
		filter.Topic = opts.Topic
	} else {
		targetName = "All Tracked Channels"
		topic = "all"
		filter.TrackedOnly = true
		filter.Topic = "all"
	}

	var rangeLabel string
	switch opts.TimeRange {
	case TimeRangeToday:
		filter.Since = now.Add(-24 * time.Hour)
		filter.Until = now
		rangeLabel = "Today (24h)"
	case TimeRangePast7Days:
		filter.Since = now.Add(-7 * 24 * time.Hour)
		filter.Until = now
		rangeLabel = "Past 7 Days"
	case TimeRangePast30Days:
		filter.Since = now.Add(-30 * 24 * time.Hour)
		filter.Until = now
		rangeLabel = "Past 30 Days"
	case TimeRangeCurrentMonth:
		filter.Since = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		filter.Until = now
		rangeLabel = now.Format("January 2006")
	case TimeRangePrevMonth:
		prevMonth := now.AddDate(0, -1, 0)
		filter.Since = time.Date(prevMonth.Year(), prevMonth.Month(), 1, 0, 0, 0, 0, now.Location())
		filter.Until = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Add(-time.Second)
		rangeLabel = prevMonth.Format("January 2006")
	default: // TimeRangeUnread
		filter.UnreadOnly = true
		rangeLabel = "Unread"
	}

	logger.Infof("Generating digest for target='%s', period='%s', topic='%s'", targetName, rangeLabel, topic)

	posts, err := s.repo.GetPosts(ctx, filter)
	if err != nil {
		logger.Errorf("Fetching posts for digest failed: %v", err)
		return nil, fmt.Errorf("fetching posts for digest: %w", err)
	}

	// If unread was selected and no local posts found, attempt a delta sync from Telegram
	if len(posts) == 0 && opts.TimeRange == TimeRangeUnread && s.tgClient != nil && s.tgClient.IsConfigured() {
		logger.Infof("No local unread posts found; triggering delta sync from Telegram...")
		_, _ = s.SyncTrackedPosts(ctx)
		posts, err = s.repo.GetPosts(ctx, filter)
		if err != nil {
			logger.Errorf("Fetching posts after sync failed: %v", err)
			return nil, fmt.Errorf("fetching posts after sync: %w", err)
		}
	}

	if len(posts) == 0 {
		errMsg := fmt.Sprintf("no posts found for '%s' in period '%s'", targetName, rangeLabel)
		if opts.TimeRange == TimeRangeUnread {
			errMsg += " (all posts are already marked read; try selecting 'Today' or 'Past 7 Days' to re-summarize, or press [s] to sync new posts)"
		}
		logger.Warnf("%s", errMsg)
		return nil, fmt.Errorf("%s", errMsg)
	}

	logger.Infof("Found %d posts for digest generation", len(posts))

	// Dynamic AI key detection (always re-check DB in case user updated Gemini key/model in TUI settings)
	apiKey, _ := s.repo.GetSetting(ctx, domain.SettingKeyGeminiAPIKey, "")
	if apiKey != "" {
		model, _ := s.repo.GetSetting(ctx, domain.SettingKeyGeminiModel, "gemini-3.5-flash")
		if prov, err := gemini.NewGeminiProvider(ctx, config.GeminiConfig{APIKey: apiKey, Model: model}); err == nil {
			s.llmProv = prov
		} else {
			logger.Errorf("Failed to initialize Gemini provider with model '%s': %v", model, err)
		}
	} else {
		s.llmProv = nil
	}

	title := fmt.Sprintf("%s - %s", targetName, rangeLabel)
	channels, _ := s.repo.ListChannels(ctx, false)

	// Load configurable digest prompt from DB (or fallback to default)
	promptTemplate, _ := s.repo.GetSetting(ctx, domain.SettingKeyDigestPrompt, domain.DefaultDigestPrompt)
	if strings.TrimSpace(promptTemplate) == "" {
		promptTemplate = domain.DefaultDigestPrompt
	}

	userInterests, _ := s.repo.GetSetting(ctx, domain.SettingKeyUserInterests, domain.DefaultUserInterests)
	userIgnored, _ := s.repo.GetSetting(ctx, domain.SettingKeyUserIgnoredTopics, domain.DefaultUserIgnoredTopics)

	// Interpolate user personal relevance profile into prompt template
	promptTemplate = strings.ReplaceAll(promptTemplate, "{{user_interests}}", userInterests)
	promptTemplate = strings.ReplaceAll(promptTemplate, "{{user_ignored_topics}}", userIgnored)

	var digest *domain.Digest
	if s.llmProv != nil && s.llmProv.HasAI() {
		logger.Infof("Invoking Gemini AI summarization engine...")
		d, err := s.llmProv.GenerateDigest(ctx, topic, posts, channels, promptTemplate)
		if err != nil {
			logger.Warnf("Gemini AI synthesis failed: %v. Gracefully falling back to Extractive mode.", err)
			extD, extErr := pipeline.GenerateExtractiveDigestWithTitle(title, topic, posts, channels)
			if extErr != nil {
				logger.Errorf("Extractive fallback also failed: %v", extErr)
				return nil, fmt.Errorf("generating digest (Gemini failed: %v; Extractive fallback failed: %w)", err, extErr)
			}
			extD.Title = fmt.Sprintf("%s (Extractive Fallback) - %s", title, time.Now().Format("Jan 02, 2006"))
			digest = extD
		} else {
			d.Title = title
			digest = d
			logger.Infof("Successfully generated AI digest: '%s' (%d posts summarized)", digest.Title, digest.PostCount)
		}
	} else {
		logger.Infof("No AI API key set. Generating digest using offline Extractive mode...")
		d, err := pipeline.GenerateExtractiveDigestWithTitle(title, topic, posts, channels)
		if err != nil {
			logger.Errorf("Generating extractive digest failed: %v", err)
			return nil, fmt.Errorf("generating extractive digest: %w", err)
		}
		digest = d
		logger.Infof("Successfully generated Extractive digest: '%s' (%d posts)", digest.Title, digest.PostCount)
	}

	// Ensure single-channel digests explicitly have the channel header at the top
	if targetChannel != nil && digest != nil {
		if !strings.Contains(digest.SummaryMarkdown, targetChannel.Title) {
			var chHeader string
			if targetChannel.Username != "" {
				chHeader = fmt.Sprintf("### 📢 [%s](https://t.me/%s) (@%s)", targetChannel.Title, targetChannel.Username, targetChannel.Username)
			} else {
				chHeader = fmt.Sprintf("### 📢 [%s](https://t.me/c/%d)", targetChannel.Title, targetChannel.ID)
			}
			digest.SummaryMarkdown = chHeader + "\n\n" + digest.SummaryMarkdown
		}
	}

	// Persist digest into database
	if err := s.repo.SaveDigest(ctx, digest); err != nil {
		return nil, fmt.Errorf("saving digest: %w", err)
	}

	// Deliver via dedicated Telegram Bot if enabled
	botEnabled, _ := s.repo.GetSetting(ctx, domain.SettingKeyDeliveryBot, "false")
	if botEnabled == "true" {
		token, _ := s.repo.GetSetting(ctx, domain.SettingKeyBotToken, s.cfg.Telegram.BotToken)
		chatID, _ := s.repo.GetSetting(ctx, domain.SettingKeyBotChatID, s.cfg.Telegram.BotChatID)
		botSender := telegram.NewBotSender(token, chatID)
		if botSender.IsConfigured() {
			if err := botSender.SendDigest(ctx, digest.SummaryMarkdown); err != nil {
				logger.Warnf("⚠️ Warning sending digest to Telegram bot: %v", err)
			} else {
				logger.Infof("✓ Successfully sent digest '%s' to Telegram bot chat %s", digest.Title, chatID)
			}
		} else {
			logger.Warnf("Telegram bot delivery enabled, but bot token or chat ID is missing")
		}
	}

	// Save to local Markdown file
	if s.cfg.Daemon.OutputDir != "" {
		_ = os.MkdirAll(s.cfg.Daemon.OutputDir, 0755)
		filename := fmt.Sprintf("digest-%s-%s.md", topic, time.Now().Format("2006-01-02-150405"))
		fullPath := filepath.Join(s.cfg.Daemon.OutputDir, filename)
		_ = os.WriteFile(fullPath, []byte(digest.SummaryMarkdown), 0644)
	}

	// Only mark posts as read when explicitly running an "Unread" digest
	if opts.TimeRange == TimeRangeUnread {
		var postIDs []int64
		for _, p := range posts {
			postIDs = append(postIDs, p.ID)
		}
		_ = s.repo.MarkPostsRead(ctx, postIDs)
	}

	return digest, nil
}

// CleanOldPosts purges read posts older than the specified duration from the local database.
func (s *FeedService) CleanOldPosts(ctx context.Context, olderThan time.Duration) (int64, error) {
	return s.repo.CleanPosts(ctx, olderThan, true)
}

// GetPostStats returns total and unread post counts from the database.
func (s *FeedService) GetPostStats(ctx context.Context) (total int, unread int, err error) {
	return s.repo.GetPostStats(ctx)
}
