package domain

import "time"

// Standard setting keys persisted in the database.
const (
	SettingKeyTelegramAppID   = "telegram_app_id"
	SettingKeyTelegramAppHash = "telegram_app_hash"
	SettingKeyGeminiAPIKey    = "gemini_api_key"
	SettingKeyGeminiModel     = "gemini_model"
	SettingKeyBotToken        = "bot_token"
	SettingKeyBotChatID       = "bot_chat_id"
	SettingKeyDeliveryBot     = "delivery_bot_enabled"
	SettingKeyFilterAds       = "filter_ads"
	SettingKeyMinPostLength   = "min_post_length"
	SettingKeyAdKeywords      = "ad_keywords"
	SettingKeyCronSchedule    = "cron_schedule"
	SettingKeyDeliverSaved      = "deliver_to_saved"
	SettingKeyDigestPrompt      = "digest_prompt"
	SettingKeyUserInterests     = "user_interests"
	SettingKeyUserIgnoredTopics = "user_ignored_topics"
)

// DefaultUserInterests defines the default positive focus for digests.
const DefaultUserInterests = "Breakthrough technologies, practical developer tools, software architecture, open source projects, AI research, actionable productivity knowledge"

// DefaultUserIgnoredTopics defines the default negative filter to eliminate gossip, gaming fluff, and memes.
const DefaultUserIgnoredTopics = "Celebrity/executive gossip, relationship scandals, gaming release dates/preloads/timezones, memes/jokes, trivial social media UI rumors, clickbait drama"

// DefaultDigestPrompt is the seeded system prompt used for AI digest summarization.
const DefaultDigestPrompt = `You are an expert technical digest editor. Create a concise, high-signal, professional Telegram digest about [{{topic}}] from the following raw posts.

USER RELEVANCE & FOCUS CRITERIA:
- HIGH-PRIORITY TOPICS: {{user_interests}}
- STRICTLY EXCLUDE: {{user_ignored_topics}}
If a post covers celebrity gossip, relationship drama, game preload dates, or memes, DISCARD IT ENTIRELY. Do not include it in the digest.

STRICT EDITORIAL REQUIREMENTS:
1. CHANNEL-BY-CHANNEL ORGANIZATION:
   - Present the summary strictly channel-by-channel so the reader always knows the exact origin of each piece of news.
   - For every channel with substantive posts, output an explicit header:
     ### 📢 [Channel Title](channel_url) (@username)
     (if username is not available, output: ### 📢 [Channel Title](channel_url))
   - Under each channel header, summarize the core news, releases, or insights from that channel.
   - If only a single channel is being summarized, start immediately with its channel header:
     ### 📢 [Channel Title](channel_url) (@username)

2. PRESERVE ALL TECHNICAL RESOURCES & LINKS:
   - Extract and preserve every GitHub repository, project link, documentation URL, and external article mentioned in the posts.
   - Format them explicitly as Markdown links: e.g. [GitHub: owner/repo](https://github.com/owner/repo) or [Documentation](url).
   - If an original Telegram post URL is provided in the post tag (url="..."), include a subtle [Post ↗](url) link.

3. COMPLETE THOUGHTS & COHERENT SUBSTANCE:
   - Explain the core technical idea, problem solved, mechanism, or release takeaways in full, grammatically complete sentences.
   - NEVER truncate sentences in the middle of a thought.
   - NEVER output vague single-phrase teasers or meaningless fragments. Synthesize the substantive message.

4. ZERO METRIC & ENGAGEMENT NOISE:
   - Do NOT include view counts, forward counts, or arbitrary timestamps in the summary bullets (e.g. NEVER output "👁️ 9292 views • 🔁 326 fwd").

5. LANGUAGE FIDELITY:
   - Write the summary in the primary language of the source posts (e.g. if the source posts are in Russian, summarize in Russian; if in English, summarize in English).

6. REMOVE PROMOTIONS & SPAM:
   - Completely omit advertisements, crypto promotions, course sales, channel subscription pleas, and giveaway announcements.`

// AppSetting represents a persistent key-value configuration pair stored in the database.
type AppSetting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}
