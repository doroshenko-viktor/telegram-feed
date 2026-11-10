package telegram

import (
	"testing"
	"unicode/utf16"

	"github.com/gotd/td/tg"
)

func TestFormatMessageWithEntities_TextURL(t *testing.T) {
	text := "Нашли интерактивный учебник, который наглядно разбирает работу 14 сетевых протоколов."

	// "интерактивный учебник" starts at index 6 in runes/utf16
	u16 := utf16.Encode([]rune(text))
	anchorU16 := utf16.Encode([]rune("интерактивный учебник"))
	prefixU16 := utf16.Encode([]rune("Нашли "))

	offset := len(prefixU16)
	length := len(anchorU16)

	if offset+length > len(u16) {
		t.Fatalf("offset+length exceeds u16 len")
	}

	entities := []tg.MessageEntityClass{
		&tg.MessageEntityTextURL{
			Offset: offset,
			Length: length,
			URL:    "https://howdns.works",
		},
	}

	formatted := FormatMessageWithEntities(text, entities)
	expected := "Нашли [интерактивный учебник](https://howdns.works), который наглядно разбирает работу 14 сетевых протоколов."
	if formatted != expected {
		t.Errorf("mismatch:\nexpected: %s\ngot:      %s", expected, formatted)
	}
}

func TestFormatMessageWithEntities_WithEmoji(t *testing.T) {
	// Emoji 🚀 takes 2 code units in UTF-16 (surrogate pair)
	text := "🚀 New release is available at GitHub repo."
	u16Prefix := utf16.Encode([]rune("🚀 New release is available at "))
	u16Anchor := utf16.Encode([]rune("GitHub repo"))

	entities := []tg.MessageEntityClass{
		&tg.MessageEntityTextURL{
			Offset: len(u16Prefix),
			Length: len(u16Anchor),
			URL:    "https://github.com/golang/go",
		},
	}

	formatted := FormatMessageWithEntities(text, entities)
	expected := "🚀 New release is available at [GitHub repo](https://github.com/golang/go)."
	if formatted != expected {
		t.Errorf("mismatch:\nexpected: %s\ngot:      %s", expected, formatted)
	}
}

func TestFormatMessageWithEntities_NoEntities(t *testing.T) {
	text := "Plain text message without any links."
	formatted := FormatMessageWithEntities(text, nil)
	if formatted != text {
		t.Errorf("expected text unchanged, got: %s", formatted)
	}
}
