package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// MaxTelegramMessageLength is the safe character limit for a single Telegram message (Telegram cap is 4096).
const MaxTelegramMessageLength = 4000

// ChunkMarkdown splits long markdown text into chunks <= maxChunkLen,
// cleanly preserving paragraph boundaries (\n\n).
func ChunkMarkdown(text string, maxChunkLen int) []string {
	if maxChunkLen <= 0 {
		maxChunkLen = MaxTelegramMessageLength
	}

	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= maxChunkLen {
		return []string{trimmed}
	}

	var chunks []string
	paragraphs := strings.Split(trimmed, "\n\n")
	var current strings.Builder

	for _, p := range paragraphs {
		pTrimmed := strings.TrimSpace(p)
		if pTrimmed == "" {
			continue
		}

		// If a single paragraph exceeds the chunk limit, split it
		if len(pTrimmed) > maxChunkLen {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
			subChunks := splitLongParagraph(pTrimmed, maxChunkLen)
			chunks = append(chunks, subChunks...)
			continue
		}

		needed := len(pTrimmed)
		if current.Len() > 0 {
			needed += 2 // "\n\n"
		}

		if current.Len()+needed > maxChunkLen {
			chunks = append(chunks, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(pTrimmed)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

func splitLongParagraph(text string, maxLen int) []string {
	var chunks []string
	lines := strings.Split(text, "\n")
	var current strings.Builder

	for _, line := range lines {
		if len(line) > maxLen {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
			for len(line) > maxLen {
				chunks = append(chunks, line[:maxLen])
				line = line[maxLen:]
			}
			if len(line) > 0 {
				current.WriteString(line)
			}
			continue
		}

		needed := len(line)
		if current.Len() > 0 {
			needed += 1 // "\n"
		}

		if current.Len()+needed > maxLen {
			chunks = append(chunks, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

// BotSender delivers messages via a dedicated Telegram Bot.
type BotSender struct {
	client   *http.Client
	botToken string
	chatID   string
}

// NewBotSender creates a new bot sender.
func NewBotSender(botToken, chatID string) *BotSender {
	return &BotSender{
		client:   &http.Client{Timeout: 15 * time.Second},
		botToken: strings.TrimSpace(botToken),
		chatID:   strings.TrimSpace(chatID),
	}
}

// IsConfigured returns true if both bot token and chat ID are provided.
func (b *BotSender) IsConfigured() bool {
	return b != nil && b.botToken != "" && b.chatID != ""
}

// SendDigest sends markdown text in chunks to the user's private chat with the bot.
func (b *BotSender) SendDigest(ctx context.Context, markdownText string) error {
	if !b.IsConfigured() {
		return fmt.Errorf("telegram bot is not configured (token and chat_id required)")
	}

	chunks := ChunkMarkdown(markdownText, MaxTelegramMessageLength)
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.botToken)

	for i, chunk := range chunks {
		payload := map[string]any{
			"chat_id":    b.chatID,
			"text":       chunk,
			"parse_mode": "Markdown",
		}
		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshaling bot message payload: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("creating http request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := b.client.Do(req)
		if err != nil {
			return fmt.Errorf("sending bot message chunk %d/%d: %w", i+1, len(chunks), err)
		}

		// Fallback to plain text if markdown parsing failed on Telegram's side
		if resp.StatusCode == http.StatusBadRequest {
			resp.Body.Close()
			payload["parse_mode"] = "" // Send as plain text
			bodyBytes, _ = json.Marshal(payload)
			req, _ = http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			resp, err = b.client.Do(req)
			if err != nil {
				return fmt.Errorf("sending fallback plain text chunk %d/%d: %w", i+1, len(chunks), err)
			}
		}

		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("telegram bot API returned status %d for chunk %d/%d", resp.StatusCode, i+1, len(chunks))
		}
	}

	return nil
}
