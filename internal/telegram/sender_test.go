package telegram

import (
	"strings"
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
