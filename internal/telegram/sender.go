package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"telegram_feed_summarizer/internal/logger"
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
	client     *http.Client
	botToken   string
	chatID     string
	apiBaseURL string
}

// NewBotSender creates a new bot sender.
func NewBotSender(botToken, chatID string) *BotSender {
	return &BotSender{
		client:     &http.Client{Timeout: 15 * time.Second},
		botToken:   strings.TrimSpace(botToken),
		chatID:     strings.TrimSpace(chatID),
		apiBaseURL: "https://api.telegram.org",
	}
}

// IsConfigured returns true if both bot token and chat ID are provided.
func (b *BotSender) IsConfigured() bool {
	return b != nil && b.botToken != "" && b.chatID != ""
}

// SendDigest sends markdown text formatted as Telegram HTML in chunks to the user's private chat with the bot.
// If the digest covers multiple channels, it sends a distinct message for each channel (or section),
// explicitly stating which channel the summary belongs to before proceeding to the next channel.
func (b *BotSender) SendDigest(ctx context.Context, markdownText string) error {
	if !b.IsConfigured() {
		return fmt.Errorf("telegram bot is not configured (token and chat_id required)")
	}

	baseURL := b.apiBaseURL
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}

	// 1. Partition digest into channel-specific message sections
	channelBlocks := SplitDigestIntoChannelMessages(markdownText)
	apiURL := fmt.Sprintf("%s/bot%s/sendMessage", baseURL, b.botToken)

	for blockIdx, block := range channelBlocks {
		// Chunk individual channel block with safe limit (3400) if any single channel exceeds limit
		chunks := ChunkMarkdown(block, 3400)

		for i, chunk := range chunks {
			htmlChunk := MarkdownToTelegramHTML(chunk)
			htmlParts := splitSafeHTML(htmlChunk, MaxTelegramMessageLength)

			for _, part := range htmlParts {
				payload := map[string]any{
					"chat_id":    b.chatID,
					"text":       part,
					"parse_mode": "HTML",
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
					return fmt.Errorf("sending bot message block %d/%d chunk %d/%d: %w", blockIdx+1, len(channelBlocks), i+1, len(chunks), err)
				}

				// If Telegram rejected HTML (400 Bad Request), log error and fall back to clean plain text
				if resp.StatusCode == http.StatusBadRequest {
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					logger.Warnf("Telegram bot API rejected HTML parse_mode (%s); falling back to clean plain text for block %d/%d", string(body), blockIdx+1, len(channelBlocks))

					plainText := StripHTMLTags(part)
					fallbackPayload := map[string]any{
						"chat_id": b.chatID,
						"text":    plainText,
					}
					bodyBytes, _ = json.Marshal(fallbackPayload)
					req, _ = http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
					req.Header.Set("Content-Type", "application/json")
					resp, err = b.client.Do(req)
					if err != nil {
						return fmt.Errorf("sending fallback plain text: %w", err)
					}
				}

				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					return fmt.Errorf("telegram bot API returned status %d for block %d/%d: %s", resp.StatusCode, blockIdx+1, len(channelBlocks), string(body))
				}

				resp.Body.Close()
				logger.Infof("Delivered digest channel message %d/%d to Telegram bot chat %s", blockIdx+1, len(channelBlocks), b.chatID)
			}
		}
	}

	return nil
}
