package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestChunkMarkdown(t *testing.T) {
	// 1. Text shorter than max limit returns single chunk
	short := "This is a short message."
	chunks := ChunkMarkdown(short, 100)
	if len(chunks) != 1 || chunks[0] != short {
		t.Fatalf("expected 1 chunk matching short text, got %v", chunks)
	}

	// 2. Text with paragraphs splits at paragraph boundaries
	p1 := strings.Repeat("A", 40)
	p2 := strings.Repeat("B", 40)
	p3 := strings.Repeat("C", 40)
	longText := p1 + "\n\n" + p2 + "\n\n" + p3

	// Limit to 60 characters -> each chunk can only hold one 40-char paragraph
	chunks = ChunkMarkdown(longText, 60)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != p1 || chunks[1] != p2 || chunks[2] != p3 {
		t.Errorf("chunk contents mismatched: %v", chunks)
	}

	// 3. Very long single paragraph splits cleanly without panic
	giant := strings.Repeat("Z", 250)
	chunks = ChunkMarkdown(giant, 100)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks for 250-char string with limit 100, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > 100 {
			t.Errorf("chunk %d exceeded limit: %d > 100", i, len(c))
		}
	}
}

func TestBotSender_NotConfigured(t *testing.T) {
	sender := NewBotSender("", "")
	err := sender.SendDigest(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error when bot is not configured")
	}
}

func TestBotSender_SendDigest_Success(t *testing.T) {
	var receivedPayloads []map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/bot12345:TEST_TOKEN/sendMessage") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		body, _ := io.ReadAll(r.Body)
		var p map[string]any
		_ = json.Unmarshal(body, &p)
		receivedPayloads = append(receivedPayloads, p)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true, "result": {"message_id": 999}}`))
	}))
	defer ts.Close()

	sender := NewBotSender("12345:TEST_TOKEN", "987654")
	sender.apiBaseURL = ts.URL

	markdown := `# 📰 Morning Digest

- **Release Announcement:** Version 2.0 is out!
  Check the repository at [GitHub: my/repo](https://github.com/my/repo).`

	err := sender.SendDigest(context.Background(), markdown)
	if err != nil {
		t.Fatalf("unexpected SendDigest error: %v", err)
	}

	if len(receivedPayloads) != 1 {
		t.Fatalf("expected 1 request, got %d", len(receivedPayloads))
	}

	req := receivedPayloads[0]
	if req["chat_id"] != "987654" {
		t.Errorf("expected chat_id 987654, got %v", req["chat_id"])
	}
	if req["parse_mode"] != "HTML" {
		t.Errorf("expected parse_mode 'HTML', got %v", req["parse_mode"])
	}
	text, _ := req["text"].(string)
	if !strings.Contains(text, "<b>📰 Morning Digest</b>") {
		t.Errorf("expected bold header in HTML text, got: %s", text)
	}
	if !strings.Contains(text, `• <b>Release Announcement:</b>`) {
		t.Errorf("expected bullet with bold title, got: %s", text)
	}
	if !strings.Contains(text, `<a href="https://github.com/my/repo">GitHub: my/repo</a>`) {
		t.Errorf("expected anchor link, got: %s", text)
	}
}

func TestBotSender_SendDigest_FallbackOn400(t *testing.T) {
	var callCount int32
	var lastPayload map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p map[string]any
		_ = json.Unmarshal(body, &p)
		lastPayload = p

		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			// Simulate Telegram rejecting HTML parse mode
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok": false, "error_code": 400, "description": "Bad Request: can't parse entities"}`))
			return
		}

		// Second call succeeds with fallback plain text
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true, "result": {"message_id": 1000}}`))
	}))
	defer ts.Close()

	sender := NewBotSender("12345:TEST_TOKEN", "987654")
	sender.apiBaseURL = ts.URL

	markdown := `### Title
Check [Doc](https://example.com) for details.`

	err := sender.SendDigest(context.Background(), markdown)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}

	if atomic.LoadInt32(&callCount) != 2 {
		t.Fatalf("expected 2 calls (initial + fallback), got %d", callCount)
	}

	// In the fallback payload, parse_mode should be empty/omitted and HTML tags should be stripped
	if lastPayload["parse_mode"] != nil && lastPayload["parse_mode"] != "" {
		t.Errorf("expected parse_mode to be empty in fallback, got %v", lastPayload["parse_mode"])
	}
	text, _ := lastPayload["text"].(string)
	if strings.Contains(text, "<b>") || strings.Contains(text, "<a") {
		t.Errorf("expected HTML tags stripped in fallback text, got: %s", text)
	}
	if !strings.Contains(text, "Doc (https://example.com)") {
		t.Errorf("expected link destination preserved as text (url), got: %s", text)
	}
}

