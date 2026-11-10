package telegram

import (
	"strings"
	"testing"
)

func TestMarkdownToTelegramHTML_HeadersAndFormatting(t *testing.T) {
	md := `# 📰 Telegram Digest: High-Signal Engineering

**Period:** Nov 09, 23:00 — Nov 10, 04:00
**Coverage:** 5 posts across 1 channel(s)

---

### 📢 [4PDA Community](https://t.me/c/123/45)

- **Как устроен интернет — теперь можно изучить на одном сайте**
  На примерах объясняют, как работает TCP и каким образом DNS находит нужный сайт в сети.
  🔗 [howdns.works](https://howdns.works) • [Telegram Post ↗](https://t.me/c/123/45)`

	result := MarkdownToTelegramHTML(md)

	// Verify headers are <b>
	if !strings.Contains(result, "<b>📰 Telegram Digest: High-Signal Engineering</b>") {
		t.Errorf("expected title to be wrapped in <b>, got:\n%s", result)
	}
	// Verify horizontal rule
	if !strings.Contains(result, "──────────") {
		t.Errorf("expected horizontal rule divider, got:\n%s", result)
	}
	// Verify bold
	if !strings.Contains(result, "<b>Period:</b>") {
		t.Errorf("expected <b>Period:</b>, got:\n%s", result)
	}
	// Verify channel link in header
	if !strings.Contains(result, `<a href="https://t.me/c/123/45">4PDA Community</a>`) {
		t.Errorf("expected channel link, got:\n%s", result)
	}
	// Verify bullet point and bold headline
	if !strings.Contains(result, "• <b>Как устроен интернет — теперь можно изучить на одном сайте</b>") {
		t.Errorf("expected bullet with bold headline, got:\n%s", result)
	}
	// Verify external links
	if !strings.Contains(result, `<a href="https://howdns.works">howdns.works</a>`) {
		t.Errorf("expected howdns.works link, got:\n%s", result)
	}
	if !strings.Contains(result, `<a href="https://t.me/c/123/45">Telegram Post ↗</a>`) {
		t.Errorf("expected Telegram Post link, got:\n%s", result)
	}
}

func TestMarkdownToTelegramHTML_EntitiesAndCode(t *testing.T) {
	md := `Check out C++ & Go comparison:
If x < 10 && y > 20 {
    log.Println("valid")
}

Here is inline code: ` + "`feed_service_worker.go`" + ` and identifier ` + "user_profile_id" + `.

` + "```go" + `
func main() {
    fmt.Println("<Hello & World>")
}
` + "```"

	result := MarkdownToTelegramHTML(md)

	// Raw text entities should be escaped
	if !strings.Contains(result, "C++ &amp; Go comparison:") {
		t.Errorf("expected & to be &amp;, got:\n%s", result)
	}
	if !strings.Contains(result, "x &lt; 10 &amp;&amp; y &gt; 20") {
		t.Errorf("expected < and > to be escaped, got:\n%s", result)
	}

	// Inline code
	if !strings.Contains(result, "<code>feed_service_worker.go</code>") {
		t.Errorf("expected <code>feed_service_worker.go</code>, got:\n%s", result)
	}

	// Identifier with underscores outside code should NOT be italicized
	if !strings.Contains(result, "user_profile_id") {
		t.Errorf("expected user_profile_id to stay untouched, got:\n%s", result)
	}
	if strings.Contains(result, "<i>profile</i>") {
		t.Errorf("user_profile_id was incorrectly italicized: %s", result)
	}

	// Code block with language
	if !strings.Contains(result, `<pre><code class="language-go">`) {
		t.Errorf("expected <pre><code class=\"language-go\">, got:\n%s", result)
	}
	if !strings.Contains(result, `&lt;Hello &amp; World&gt;`) {
		t.Errorf("expected escaped content inside code block, got:\n%s", result)
	}
}

func TestMarkdownToTelegramHTML_BlockquotesAndNestedLinks(t *testing.T) {
	md := `> First line of wisdom
> Second line with a link to [Wikipedia: Go (language)](https://en.wikipedia.org/wiki/Go_(programming_language))

Outside the quote.`

	result := MarkdownToTelegramHTML(md)

	if !strings.Contains(result, "<blockquote>") || !strings.Contains(result, "</blockquote>") {
		t.Errorf("expected blockquote, got:\n%s", result)
	}
	if !strings.Contains(result, `<a href="https://en.wikipedia.org/wiki/Go_(programming_language)">Wikipedia: Go (language)</a>`) {
		t.Errorf("expected link with parentheses, got:\n%s", result)
	}
}

func TestBalanceHTMLTags(t *testing.T) {
	// Unclosed bold
	unclosed := "<b>Some bold text without closing"
	balanced := balanceHTMLTags(unclosed)
	if balanced != "<b>Some bold text without closing</b>" {
		t.Errorf("expected closed </b>, got %s", balanced)
	}

	// Multiple unclosed nested
	nested := "<blockquote><b><i>Deep nested"
	balancedNested := balanceHTMLTags(nested)
	if balancedNested != "<blockquote><b><i>Deep nested</i></b></blockquote>" {
		t.Errorf("expected properly closed tags in reverse order, got %s", balancedNested)
	}

	// Already balanced
	ok := "<b>ok</b> <i>fine</i>"
	if balanceHTMLTags(ok) != ok {
		t.Errorf("expected unchanged, got %s", balanceHTMLTags(ok))
	}
}

func TestStripHTMLTags(t *testing.T) {
	input := `<b>Headline</b>: Check <a href="https://example.com">Site &amp; Co</a> for &lt;info&gt;!`
	plain := StripHTMLTags(input)
	expected := "Headline: Check Site & Co (https://example.com) for <info>!"
	if plain != expected {
		t.Errorf("expected %q, got %q", expected, plain)
	}
}

func TestSplitSafeHTML(t *testing.T) {
	p1 := "<b>Paragraph 1: " + strings.Repeat("A", 40) // Unclosed bold
	p2 := "<b>Paragraph 2: " + strings.Repeat("B", 40) // Unclosed bold
	p3 := "<b>Paragraph 3: " + strings.Repeat("C", 40) // Unclosed bold
	combined := p1 + "\n\n" + p2 + "\n\n" + p3

	parts := splitSafeHTML(combined, 65)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}

	for i, part := range parts {
		if !strings.HasPrefix(part, "<b>") || !strings.HasSuffix(part, "</b>") {
			t.Errorf("part %d not properly tag-balanced: %s", i, part)
		}
	}
}
