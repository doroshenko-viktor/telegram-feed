package telegram

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/gotd/td/tg"
)

type textUrlEntity struct {
	offset int
	length int
	url    string
}

// FormatMessageWithEntities converts Telegram message entities (specifically MessageEntityTextUrl)
// into Markdown hyperlinks so embedded links are preserved in local storage and LLM prompts.
func FormatMessageWithEntities(text string, entities []tg.MessageEntityClass) string {
	if len(entities) == 0 || text == "" {
		return text
	}

	u16 := utf16.Encode([]rune(text))
	u16Len := len(u16)

	// Collect and sort text URL entities
	var urlEntities []textUrlEntity
	for _, ent := range entities {
		if textUrl, ok := ent.(*tg.MessageEntityTextURL); ok {
			if textUrl.Offset >= 0 && textUrl.Offset+textUrl.Length <= u16Len && textUrl.Length > 0 && textUrl.URL != "" {
				urlEntities = append(urlEntities, textUrlEntity{
					offset: textUrl.Offset,
					length: textUrl.Length,
					url:    strings.TrimSpace(textUrl.URL),
				})
			}
		}
	}

	if len(urlEntities) == 0 {
		return text
	}

	// Sort entities by offset ascending
	sort.Slice(urlEntities, func(i, j int) bool {
		return urlEntities[i].offset < urlEntities[j].offset
	})

	var sb strings.Builder
	currIdx := 0

	for _, ent := range urlEntities {
		if ent.offset < currIdx {
			// Overlapping entity, skip
			continue
		}

		// Write unformatted slice before this entity
		if ent.offset > currIdx {
			sb.WriteString(string(utf16.Decode(u16[currIdx:ent.offset])))
		}

		anchor := string(utf16.Decode(u16[ent.offset : ent.offset+ent.length]))
		// If anchor is already a raw URL, avoid [http://...](http://...)
		if strings.HasPrefix(anchor, "http://") || strings.HasPrefix(anchor, "https://") {
			sb.WriteString(anchor)
		} else {
			sb.WriteString(fmt.Sprintf("[%s](%s)", anchor, ent.url))
		}

		currIdx = ent.offset + ent.length
	}

	// Write remaining tail
	if currIdx < u16Len {
		sb.WriteString(string(utf16.Decode(u16[currIdx:])))
	}

	return sb.String()
}
