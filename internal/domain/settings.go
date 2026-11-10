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
)

// AppSetting represents a persistent key-value configuration pair stored in the database.
type AppSetting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}
