-- PostgreSQL Schema
CREATE TABLE IF NOT EXISTS telegram_sessions (
    id TEXT PRIMARY KEY,
    data BYTEA NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ad_patterns (
    id BIGSERIAL PRIMARY KEY,
    pattern TEXT NOT NULL UNIQUE,
    is_regex BOOLEAN NOT NULL DEFAULT FALSE,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed default ad patterns into PostgreSQL
INSERT INTO ad_patterns (pattern, is_regex, description) VALUES
    ('#реклама', FALSE, 'Standard Russian ad hashtag'),
    ('#ad', FALSE, 'Standard English ad hashtag'),
    ('#sponsored', FALSE, 'Sponsored content tag'),
    ('#партнерский', FALSE, 'Partner content tag'),
    ('t\.me\/(?:joinchat|\+|addlist)\/[a-zA-Z0-9_\-]+', TRUE, 'Telegram invite link'),
    ('(?:ref|aff|promo)=[a-zA-Z0-9_\-]+', TRUE, 'Referral and affiliate link'),
    ('на правах рекламы', FALSE, 'Ad marker phrase'),
    ('скидк[аеи] \d+%', TRUE, 'Discount announcement pattern'),
    ('сигналы крипт', FALSE, 'Crypto signals spam')
ON CONFLICT (pattern) DO NOTHING;

CREATE TABLE IF NOT EXISTS channels (
    id BIGINT PRIMARY KEY,
    access_hash BIGINT NOT NULL,
    title TEXT NOT NULL,
    username TEXT,
    about TEXT,
    is_tracked BOOLEAN NOT NULL DEFAULT FALSE,
    mcp_allowed BOOLEAN NOT NULL DEFAULT FALSE,
    filter_ads BOOLEAN NOT NULL DEFAULT TRUE,
    summary TEXT,
    topics TEXT,
    content_type TEXT DEFAULT 'general',
    language TEXT DEFAULT 'en',
    last_sync_msg_id INT NOT NULL DEFAULT 0,
    profiled_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS posts (
    id BIGSERIAL PRIMARY KEY,
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    msg_id INT NOT NULL,
    date TIMESTAMP WITH TIME ZONE NOT NULL,
    text TEXT NOT NULL,
    views INT NOT NULL DEFAULT 0,
    forwards INT NOT NULL DEFAULT 0,
    has_media BOOLEAN NOT NULL DEFAULT FALSE,
    is_ad BOOLEAN NOT NULL DEFAULT FALSE,
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(channel_id, msg_id)
);

CREATE TABLE IF NOT EXISTS digests (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    topic TEXT NOT NULL DEFAULT 'all',
    period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    channel_ids TEXT NOT NULL,
    summary_markdown TEXT NOT NULL,
    post_count INT NOT NULL DEFAULT 0,
    mcp_visible BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_channels_mcp ON channels(mcp_allowed);
CREATE INDEX IF NOT EXISTS idx_channels_tracked ON channels(is_tracked);
CREATE INDEX IF NOT EXISTS idx_posts_channel_date ON posts(channel_id, date);
CREATE INDEX IF NOT EXISTS idx_posts_is_ad ON posts(is_ad);
CREATE INDEX IF NOT EXISTS idx_posts_is_read ON posts(is_read);
CREATE INDEX IF NOT EXISTS idx_digests_mcp ON digests(mcp_visible, created_at);
