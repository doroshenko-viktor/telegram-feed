# 📡 telegram-feed

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Protocol: MCP](https://img.shields.io/badge/Protocol-MCP-purple.svg)](https://modelcontextprotocol.io)

An extensible, production-ready Go application that connects to your personal Telegram account via MTProto, filters out channel noise and ads, and turns overwhelming Telegram programming feeds into high-signal digests.

Built as a **single, zero-configuration static binary** with:
- **Interactive Terminal UI (TUI)** (powered by [Bubble Tea](https://github.com/charmbracelet/bubbletea)).
- **Model Context Protocol (MCP) Server** (for Claude Desktop, Cursor, Antigravity, Open WebUI).
- **Zero-Trust Privacy Gateway**: third-party LLMs physically only see channels you explicitly allowlist.
- **Database-Backed MTProto Sessions**: zero session files on disk; stored directly in SQLite or PostgreSQL.
- **Dynamic Database-Driven Ad Filtering**: customizable regex and keyword ad rules editable directly via TUI/MCP.
- **Cross-Platform**: pure Go without CGO, kernel locks, or OS-specific domain sockets.

---

## ⚡ Quickstart

### 1. Build or Install

Using **mise** (recommended):
```bash
# Build binary into ./bin/telegram-feed
mise run build

# Install globally to ~/.local/bin/telegram-feed
mise run install
```
*(Or with standard Go: `go install ./cmd/telegram-feed`)*

### 2. Configure Credentials

`telegram-feed` connects via Telegram's native **MTProto** protocol to access channel feeds. This requires your own Telegram `app_id` and `app_hash`:

1. Go to **[https://my.telegram.org](https://my.telegram.org)** and log in with your phone number.
2. Click on **API development tools**.
3. Fill out the application form (e.g. App title: `TelegramFeed`, Short name: `tgfeed`, Platform: `Desktop`).
4. Copy the resulting **`App api_id`** (numeric) and **`App api_hash`** (32-character string).

Set them via environment variables:
```bash
export TELEGRAM_APP_ID=12345678
export TELEGRAM_APP_HASH="abcdef0123456789abcdef0123456789"
```
*(Alternatively, create `~/.config/telegram-feed/config.yaml` using [config.example.yaml](config.example.yaml)).*

### 3. Log In (Once)
```bash
mise run login
# or: ./bin/telegram-feed login
```
The interactive terminal wizard will prompt you for:
- Your phone number (with country code, e.g. `+1...` or `+44...`).
- The login verification code sent to your Telegram app.
- Your 2-step verification (2FA) cloud password (if enabled on your account).

Once authenticated, your session is securely persisted directly into the database (`telegram_sessions` table). You will not need to log in again.

### 4. Open the TUI
```bash
mise run tui
# or: ./bin/telegram-feed
```
Browse joined channels, press `Space` to track, and press `m` to allowlist channels for MCP.

---

## 🛠️ Mise Tasks Reference

| Task | Command | Description |
| :--- | :--- | :--- |
| `mise run build` | `go build ...` | Compiles single static binary into `./bin/telegram-feed` |
| `mise run install` | `install ...` | Installs static binary globally to `~/.local/bin/telegram-feed` |
| `mise run test` | `go test -v -race ./...` | Runs full automated test suite with race detection |
| `mise run tui` | `go run ./cmd/...` | Launches interactive Terminal User Interface (TUI) |
| `mise run login` | `go run ./cmd/... login` | Interactive Telegram MTProto terminal login wizard |
| `mise run sync` | `go run ./cmd/... sync` | Discovers joined channels and syncs latest messages |
| `mise run mcp` | `go run ./cmd/... mcp` | Starts Model Context Protocol (MCP) server over stdio |
| `mise run digest` | `go run ./cmd/... digest` | Generates on-demand topic digest |
| `mise run daemon` | `go run ./cmd/... daemon` | Runs background cron worker |
| `mise run config` | `go run ./cmd/... config` | Inspects active configuration and capabilities |
| `mise run build-all` | `GOOS=... go build ...` | Cross-compiles binaries for macOS, Linux, and Windows |

---

## 🎮 TUI Keybindings

| Key | Action |
| :--- | :--- |
| `↑` / `↓` (or `k` / `j`) | Navigate channels or rules |
| `Space` | Toggle Sync tracking (`is_tracked`) |
| `m` | **Zero-Trust Privacy**: Toggle third-party LLM access over MCP (`mcp_allowed`) |
| `a` | Toggle Ad Filtering for selected channel |
| `/` | Search & filter channels by title, username, or topic tag |
| `o` | Open **Settings & Dynamic Ad Rules** modal |
| `d` | Browse **Past Digests History** |
| `r` | Refresh database records |
| `q` / `Ctrl+C` | Quit |

---

## 🤖 MCP Server Setup (Claude Desktop & Cursor)

`telegram-feed` includes a native Model Context Protocol (MCP) server. The host LLM in your chat analyzes, synthesizes, and queries posts directly in the conversation—**requiring zero local LLM API keys**.

Add to your `claude_desktop_config.json`:
```json
{
  "mcpServers": {
    "telegram-feed": {
      "command": "/path/to/telegram-feed",
      "args": ["mcp"],
      "env": {
        "TELEGRAM_APP_ID": "12345678",
        "TELEGRAM_APP_HASH": "your_app_hash_here"
      }
    }
  }
}
```

### Available MCP Tools

| Tool | Description |
| :--- | :--- |
| `telegram_list_channels` | Lists channels allowlisted for LLM access. Excludes quarantined/private channels. |
| `telegram_get_unread_posts` | Fetches clean, unread posts filtered by topic or channel (respecting ad filter). |
| `telegram_search_posts` | Full-text search across messages in allowlisted channels. |
| `telegram_get_past_digests` | Recalls previously generated digests from cache without reading raw messages. |
| `telegram_save_digest` | Persists a new digest generated by the LLM into the database. |
| `telegram_get_settings` | Inspects active settings (`filter_ads`, etc.). |
| `telegram_update_setting` | Updates configuration settings dynamically. |
| `telegram_list_ad_patterns` | Lists dynamic noise and ad rules stored in the database. |
| `telegram_add_ad_pattern` | Adds a new keyword or regex rule to filter promo messages. |

---

## 🛡️ Zero-Trust MCP Privacy Gateway

Your Telegram account may contain private groups, family chats, or sensitive channels.
`telegram-feed` implements **hard database-level isolation**:
1. Channels have `mcp_allowed = false` by default.
2. In the TUI, you explicitly press `m` to expose a channel to MCP.
3. Every MCP query runs through:
   ```sql
   SELECT p.* FROM posts p
   JOIN channels c ON p.channel_id = c.id
   WHERE c.mcp_allowed = TRUE
   ```
4. Even if an external prompt attempts to query an unapproved channel ID, the `PrivacyGuard` layer strips it before querying.
5. Digests containing any quarantined channel are marked `mcp_visible = false` and cannot be retrieved over MCP.

---

## 🗄️ Multi-Database Support (SQLite & PostgreSQL)

- **SQLite (Default / Zero-Config)**: Automatically creates and migrates `~/.config/telegram-feed/feed.db` in WAL mode on startup.
- **PostgreSQL**: Reuses your existing server. Simply provide a PostgreSQL connection URL:
  ```bash
  export STORAGE_DSN="postgres://user:password@localhost:5432/telegram_feed?sslmode=disable"
  ```
  The binary automatically detects Postgres from the URL prefix and executes Postgres migrations.

---

## 🤖 Optional Telegram Bot Delivery

By default, digests are pull-based via your LLM chat (MCP) to avoid polluting your messages.
If you prefer autonomous push digests:
1. Create a bot with [@BotFather](https://t.me/BotFather) and get a bot token.
2. Get your numeric chat ID with [@userinfobot](https://t.me/userinfobot).
3. Set in TUI Settings (`o`) or via MCP:
   - `bot_token`: `"123456:ABC-DEF..."`
   - `bot_chat_id`: `"987654321"`
   - `delivery_bot_enabled`: `"true"`

The daemon or CLI will deliver formatted markdown digests directly into your private chat with the bot, cleanly chunked under 4,000 characters.

---

## 🧪 Testing

Run the complete test suite with race detector:
```bash
mise run test
# or: go test -v -race ./...
```

---

## 📄 License
MIT
