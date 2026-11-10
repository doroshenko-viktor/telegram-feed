package telegram

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	// Code blocks: ```[lang]\n...\n```
	codeBlockRegex = regexp.MustCompile("(?s)```([a-zA-Z0-9_-]*)\n?([\\s\\S]*?)```")
	// Inline code: `...`
	inlineCodeRegex = regexp.MustCompile("`([^`\n]+)`")
	// Headers: # Title, ## Subtitle, etc.
	headerRegex = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
	// Horizontal rules: ---, ***, ___
	hrRegex = regexp.MustCompile(`(?m)^(\s*[-*_]\s*){3,}$`)
	// Bullet points: - item, * item, + item
	bulletRegex = regexp.MustCompile(`(?m)^(\s*)[-*+]\s+(.+)$`)
	// Bold: **text** or __text__
	boldAsteriskRegex   = regexp.MustCompile(`\*\*([^\*\n]+?)\*\*`)
	boldUnderscoreRegex = regexp.MustCompile(`__([^_\n]+?)__`)
	// Italic: *text* (single asterisk)
	italicAsteriskRegex = regexp.MustCompile(`\*([^\*\n\s][^\*\n]*?[^\*\n\s]|\S)\*`)
	// Italic: _text_ (single underscore with word boundaries)
	italicUnderscoreRegex = regexp.MustCompile(`(^|[\s\(\[\{])_([^\s_][^_]*?[^\s_]|[^_\s])_($|[\s\)\]\}\.\,\!\?])`)
	// Strikethrough: ~~text~~
	strikeRegex = regexp.MustCompile(`~~([^~\n]+?)~~`)
	// Generic HTML tag regex for stripping
	htmlTagRegex = regexp.MustCompile(`(?s)<[^>]*>`)
	// Link tag regex for stripping
	linkTagRegex = regexp.MustCompile(`(?s)<a\s+[^>]*href=["']([^"']*)["'][^>]*>(.*?)</a>`)
)

// MarkdownToTelegramHTML converts standard Markdown into Telegram Bot API compliant HTML.
// It ensures that:
// 1. Text entities (&, <, >) outside tags are properly escaped.
// 2. Bold, italics, headers, bullets, blockquotes, and links are converted to Telegram HTML.
// 3. Code blocks and inline code preserve raw characters and are safely escaped.
// 4. Underscores in identifiers (e.g. foo_bar_baz) are NOT mangled into italics.
// 5. All opened tags are balanced and cleanly closed.
func MarkdownToTelegramHTML(md string) string {
	if strings.TrimSpace(md) == "" {
		return ""
	}

	// 1. Extract code blocks and inline code to protect them from formatting transformations
	var codeBlocks []string
	md = codeBlockRegex.ReplaceAllStringFunc(md, func(match string) string {
		sub := codeBlockRegex.FindStringSubmatch(match)
		lang := ""
		content := match
		if len(sub) >= 3 {
			lang = strings.TrimSpace(sub[1])
			content = sub[2]
		}
		escapedCode := html.EscapeString(content)
		var blockHTML string
		if lang != "" {
			blockHTML = fmt.Sprintf("<pre><code class=\"language-%s\">%s</code></pre>", lang, escapedCode)
		} else {
			blockHTML = fmt.Sprintf("<pre>%s</pre>", escapedCode)
		}
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, blockHTML)
		return fmt.Sprintf("\x00CODEBLOCK_%d\x00", idx)
	})

	var inlineCodes []string
	md = inlineCodeRegex.ReplaceAllStringFunc(md, func(match string) string {
		sub := inlineCodeRegex.FindStringSubmatch(match)
		content := ""
		if len(sub) >= 2 {
			content = sub[1]
		}
		escapedInline := html.EscapeString(content)
		inlineHTML := fmt.Sprintf("<code>%s</code>", escapedInline)
		idx := len(inlineCodes)
		inlineCodes = append(inlineCodes, inlineHTML)
		return fmt.Sprintf("\x00INLINECODE_%d\x00", idx)
	})

	// 2. Escape raw HTML entities in the remaining text
	md = html.EscapeString(md)

	// 3. Block-level transformations
	// 3a. Horizontal rules: ---, ***, ___
	md = hrRegex.ReplaceAllString(md, "──────────")

	// 3b. Headings: `# Title` -> `<b>Title</b>`
	md = headerRegex.ReplaceAllStringFunc(md, func(match string) string {
		sub := headerRegex.FindStringSubmatch(match)
		if len(sub) >= 3 {
			title := strings.TrimSpace(sub[2])
			return fmt.Sprintf("<b>%s</b>", title)
		}
		return match
	})

	// 3c. Bullet lists: `- item`, `* item`, `+ item` -> `• item`
	md = bulletRegex.ReplaceAllString(md, "$1• $2")

	// 3d. Blockquotes: lines starting with `&gt; `
	md = convertBlockquotes(md)

	// 4. Inline formatting
	// 4a. Links: [text](url) -> <a href="url">text</a>
	md = convertMarkdownLinks(md)

	// 4b. Bold: **text** and __text__
	md = boldAsteriskRegex.ReplaceAllString(md, "<b>$1</b>")
	md = boldUnderscoreRegex.ReplaceAllString(md, "<b>$1</b>")

	// 4c. Strikethrough: ~~text~~
	md = strikeRegex.ReplaceAllString(md, "<s>$1</s>")

	// 4d. Italic: *text* and _text_ (with boundaries)
	md = italicAsteriskRegex.ReplaceAllString(md, "<i>$1</i>")
	md = italicUnderscoreRegex.ReplaceAllString(md, "$1<i>$2</i>$3")

	// 5. Restore code placeholders
	for i, block := range codeBlocks {
		placeholder := fmt.Sprintf("\x00CODEBLOCK_%d\x00", i)
		md = strings.ReplaceAll(md, placeholder, block)
	}
	for i, inline := range inlineCodes {
		placeholder := fmt.Sprintf("\x00INLINECODE_%d\x00", i)
		md = strings.ReplaceAll(md, placeholder, inline)
	}

	// 6. Ensure all tags are properly balanced and closed
	return balanceHTMLTags(md)
}

// convertMarkdownLinks converts [text](url) to <a href="url">text</a> supporting nested parentheses in URLs.
func convertMarkdownLinks(s string) string {
	var b strings.Builder
	i := 0
	n := len(s)

	for i < n {
		if s[i] == '[' {
			// Find matching ']'
			closeBracket := -1
			for j := i + 1; j < n; j++ {
				if s[j] == '\n' {
					break
				}
				if s[j] == ']' {
					closeBracket = j
					break
				}
			}

			if closeBracket != -1 && closeBracket+1 < n && s[closeBracket+1] == '(' {
				// Find matching ')' with nested parenthesis tracking
				openParen := closeBracket + 1
				closeParen := -1
				parenDepth := 1
				for j := openParen + 1; j < n; j++ {
					if s[j] == '\n' {
						break
					}
					if s[j] == '(' {
						parenDepth++
					} else if s[j] == ')' {
						parenDepth--
						if parenDepth == 0 {
							closeParen = j
							break
						}
					}
				}

				if closeParen != -1 {
					anchor := s[i+1 : closeBracket]
					rawURL := s[openParen+1 : closeParen]
					trimmedURL := strings.TrimSpace(rawURL)
					if strings.HasPrefix(trimmedURL, "http://") || strings.HasPrefix(trimmedURL, "https://") || strings.HasPrefix(trimmedURL, "tg://") {
						safeURL := strings.ReplaceAll(trimmedURL, `"`, "&quot;")
						b.WriteString(fmt.Sprintf(`<a href="%s">%s</a>`, safeURL, anchor))
						i = closeParen + 1
						continue
					}
				}
			}
		}

		b.WriteByte(s[i])
		i++
	}

	return b.String()
}

// convertBlockquotes groups consecutive lines starting with &gt; into <blockquote> blocks.
func convertBlockquotes(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	var inQuote bool
	var quoteLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "&gt;") {
			quoteContent := strings.TrimPrefix(trimmed, "&gt;")
			quoteContent = strings.TrimPrefix(quoteContent, " ")
			quoteLines = append(quoteLines, quoteContent)
			inQuote = true
		} else {
			if inQuote {
				result = append(result, "<blockquote>"+strings.Join(quoteLines, "\n")+"</blockquote>")
				quoteLines = nil
				inQuote = false
			}
			result = append(result, line)
		}
	}
	if inQuote {
		result = append(result, "<blockquote>"+strings.Join(quoteLines, "\n")+"</blockquote>")
	}

	return strings.Join(result, "\n")
}

// balanceHTMLTags tracks opened Telegram HTML tags and closes any remaining unclosed tags at the end.
func balanceHTMLTags(s string) string {
	tagRegex := regexp.MustCompile(`</?([a-zA-Z0-9_-]+)(?:\s+[^>]*)?>`)
	matches := tagRegex.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s
	}

	var stack []string
	for _, m := range matches {
		fullTag := s[m[0]:m[1]]
		tagName := strings.ToLower(s[m[2]:m[3]])

		switch tagName {
		case "b", "strong", "i", "em", "u", "ins", "s", "strike", "del", "a", "code", "pre", "blockquote", "tg-spoiler":
			if strings.HasPrefix(fullTag, "</") {
				// Closing tag: if matches top of stack, pop
				if len(stack) > 0 && stack[len(stack)-1] == tagName {
					stack = stack[:len(stack)-1]
				}
			} else if !strings.HasSuffix(fullTag, "/>") {
				// Opening tag: push to stack
				stack = append(stack, tagName)
			}
		}
	}

	// If there are unclosed tags, close them in reverse order
	if len(stack) > 0 {
		var closing strings.Builder
		for i := len(stack) - 1; i >= 0; i-- {
			closing.WriteString("</" + stack[i] + ">")
		}
		s += closing.String()
	}

	return s
}

// StripHTMLTags removes HTML tags and unescapes HTML entities, preserving link URLs in parentheses.
func StripHTMLTags(s string) string {
	// First, transform <a href="url">text</a> into text (url) so links are preserved
	s = linkTagRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := linkTagRegex.FindStringSubmatch(match)
		if len(sub) >= 3 {
			href := sub[1]
			text := htmlTagRegex.ReplaceAllString(sub[2], "")
			if text == "" || text == href {
				return href
			}
			return fmt.Sprintf("%s (%s)", text, href)
		}
		return match
	})

	// Remove all other HTML tags
	s = htmlTagRegex.ReplaceAllString(s, "")

	// Unescape HTML entities (&amp; -> &, &lt; -> <, &gt; -> >, etc.)
	s = html.UnescapeString(s)

	return s
}

// splitSafeHTML splits HTML text if it exceeds maxLen, ensuring all HTML tags are balanced across splits.
func splitSafeHTML(htmlText string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = MaxTelegramMessageLength
	}
	if len(htmlText) <= maxLen {
		return []string{htmlText}
	}

	var parts []string
	paragraphs := strings.Split(htmlText, "\n\n")
	var current strings.Builder

	for _, p := range paragraphs {
		pTrimmed := strings.TrimSpace(p)
		if pTrimmed == "" {
			continue
		}

		needed := len(pTrimmed)
		if current.Len() > 0 {
			needed += 2 // "\n\n"
		}

		if current.Len()+needed > maxLen && current.Len() > 0 {
			parts = append(parts, balanceHTMLTags(current.String()))
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(pTrimmed)
	}

	if current.Len() > 0 {
		parts = append(parts, balanceHTMLTags(current.String()))
	}

	return parts
}

// SplitDigestIntoChannelMessages splits a multi-channel digest into distinct messages per channel.
// If the digest contains multiple channel sections (delimited by "📢"), each channel section
// becomes its own message. If there is an introductory overview before the first channel,
// it is returned as the first message.
func SplitDigestIntoChannelMessages(markdownText string) []string {
	trimmed := strings.TrimSpace(markdownText)
	if trimmed == "" {
		return nil
	}

	lines := strings.Split(trimmed, "\n")
	var rawSections []string
	var current strings.Builder

	isChannelHeader := func(line string) bool {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			t = strings.TrimLeft(t, "#")
			t = strings.TrimSpace(t)
		}
		return strings.HasPrefix(t, "📢")
	}

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		// Skip standalone horizontal rules right before a channel header
		if hrRegex.MatchString(trimmedLine) {
			continue
		}

		if isChannelHeader(line) && current.Len() > 0 {
			content := strings.TrimSpace(current.String())
			if content != "" {
				rawSections = append(rawSections, content)
			}
			current.Reset()
		}

		current.WriteString(line)
		current.WriteString("\n")
	}

	if current.Len() > 0 {
		content := strings.TrimSpace(current.String())
		if content != "" {
			rawSections = append(rawSections, content)
		}
	}

	if len(rawSections) <= 1 {
		return []string{trimmed}
	}

	// If the very first section is just a short 1-2 line title without channel or substantive bullets,
	// merge it into the first channel section so we don't send a trivial 1-line message.
	firstSec := rawSections[0]
	if !isChannelHeader(firstSec) {
		linesCount := len(strings.Split(firstSec, "\n"))
		hasBullets := strings.Contains(firstSec, "•") || strings.Contains(firstSec, "- ") || strings.Contains(firstSec, "* ")
		if linesCount <= 3 && !hasBullets && len(rawSections) > 1 {
			rawSections[1] = firstSec + "\n\n" + rawSections[1]
			rawSections = rawSections[1:]
		}
	}

	return rawSections
}
