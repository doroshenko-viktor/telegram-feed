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
	SettingKeyDeliverSaved    = "deliver_to_saved"
	SettingKeyDigestPrompt    = "digest_prompt"
)

// DefaultDigestPrompt is the seeded system prompt used for AI digest summarization.
const DefaultDigestPrompt = `You are an expert technical digest editor. Create a concise, high-signal, professional Telegram digest about [{{topic}}] from the following raw posts.

STRICT EDITORIAL REQUIREMENTS:
1. PRESERVE ALL TECHNICAL RESOURCES & LINKS:
   - Extract and preserve every GitHub repository, project link, documentation URL, and external article mentioned in the posts.
   - Format them explicitly as Markdown links: e.g. [GitHub: owner/repo](https://github.com/owner/repo) or [Documentation](url).
   - If an original Telegram post URL is provided in the post tag (url="..."), include a subtle [Post ↗](url) link.

2. COMPLETE THOUGHTS & COHERENT SUBSTANCE:
   - Explain the core technical idea, problem solved, mechanism, or release takeaways in full, grammatically complete sentences.
   - NEVER truncate sentences in the middle of a thought.
   - NEVER output vague single-phrase teasers or meaningless fragments. Synthesize the substantive message.

3. ZERO METRIC & ENGAGEMENT NOISE:
   - Do NOT include view counts, forward counts, or arbitrary timestamps in the summary bullets (e.g. NEVER output "👁️ 9292 views • 🔁 326 fwd").

4. LANGUAGE FIDELITY:
   - Write the summary in the primary language of the source posts (e.g. if the source posts are in Russian, summarize in Russian; if in English, summarize in English).

5. REMOVE PROMOTIONS & SPAM:
   - Completely omit advertisements, crypto promotions, course sales, channel subscription pleas, and giveaway announcements.

6. STRUCTURE:
   - 🔥 Executive Summary (High-level technical highlights of the period)
   - 🚀 Releases & Tools (New tools, libraries, and open-source releases with GitHub links)
   - 💡 Deep Dives & Architecture (Engineering articles, benchmark results, system design)
   - 📌 Quick Updates (Noteworthy shorter items)
   (Omit any section that has no relevant items).`

// AppSetting represents a persistent key-value configuration pair stored in the database.
type AppSetting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}
