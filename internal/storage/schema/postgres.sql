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

INSERT INTO settings (key, value) VALUES (
    'digest_prompt',
    'You are an expert technical digest editor. Create a concise, high-signal, professional Telegram digest about [{{topic}}] from the following raw posts.

USER RELEVANCE & FOCUS CRITERIA:
- HIGH-PRIORITY TOPICS: {{user_interests}}
- STRICTLY EXCLUDE: {{user_ignored_topics}}
If a post covers celebrity gossip, relationship drama, game preload dates, or memes, DISCARD IT ENTIRELY. Do not include it in the digest.

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
   (Omit any section that has no relevant items).'
) ON CONFLICT (key) DO NOTHING;

INSERT INTO settings (key, value) VALUES
    ('user_interests', 'Breakthrough technologies, practical developer tools, software architecture, open source projects, AI research, actionable productivity knowledge'),
    ('user_ignored_topics', 'Celebrity/executive gossip, relationship scandals, gaming release dates/preloads/timezones, memes/jokes, trivial social media UI rumors, clickbait drama')
ON CONFLICT (key) DO NOTHING;

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
