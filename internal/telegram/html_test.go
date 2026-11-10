package telegram

import (
	"strings"
	"testing"
)

func TestMarkdownToTelegramHTML_UserRealExample(t *testing.T) {
	md := `🔥 **Executive Summary**

Главными событиями дня стали анонс новых моделей от Anthropic (Claude Fable 5.2 и Mythos 5.2), зафиксированное падение трафика Stack Overflow в эпоху генеративного ИИ, а также масштабное расследование вокруг Nvidia и соблюдения экспортных ограничений на поставку ИИ-чипов.

---

🚀 **Релизы и инструменты**

* **Anthropic представила модели Claude Fable 5.2 и Claude Mythos 5.2:** По заявлению разработчиков, новые модели превосходят Opus 5.5 по большинству ключевых метрик. Fable 5.2 генерирует ответы на 30% быстрее предшественников (стоимость API составляет $8 за 1M входных и $40 за 1M выходных токенов). Модель Mythos 5.2 на данном этапе доступна только организациям по специальному разрешению.
* **Субвокальный ввод текста от Subvocal:** Стартап Subvocal продемонстрировал прототип умных очков, позволяющих вводить текст практически беззвучно. Нейросеть распознаёт микродвижения мышц лица и губ, обеспечивая скорость набора до 200 слов в минуту.
* **Интерактивный справочник по сетевым протоколам:** Запущен наглядный веб-учебник, разбирающий работу 14 сетевых протоколов (включая детальные механизмы TCP и работу DNS-маршрутизации).

---

💡 **ИТ-индустрия и аналитика**

* **Закат эпохи Stack Overflow:** Популярность чат-ботов и ИИ-ассистентов привела к критическому падению активности на платформе — количество новых вопросов снизилось до менее чем 1000 в месяц. Для сравнения, в 2014 году показатели превышали 200 тысяч вопросов ежемесячно.
* **Расследование в отношении главы Nvidia:** Журналистские расследования и данные утечек американских служб безопасности привели к назначению закрытого брифинга в Конгрессе США на 18 ноября. Расследование касается возможного обхода экспортных ограничений на поставку передовых ИИ-ускорителей в Китай через сторонние азиатские структуры (в частности, сингапурскую Megaspeed).

---

📌 **Коротко**

* **Игровая индустрия:** Стартовала предзагрузка GTA 6 на консолях PS5 и Xbox Series; одновременно анонсирован джейлбрейк для прошивки PS5 14.22. Анонсирована cRPG *Warhammer 40,000: Dark Heresy* — релиз запланирован на 28 января.
* **Медиа:** Kojima Productions совместно со студией E&H Production анонсировали аниме-сериал *Death Stranding Isolations*. Премьера на Disney+ намечена на 2027 год.`

	html := MarkdownToTelegramHTML(md)
	t.Logf("Converted HTML:\n%s", html)

	// Verify no raw unrendered double asterisks remain
	if strings.Contains(html, "**") {
		t.Errorf("expected no double asterisks in HTML, but found some:\n%s", html)
	}

	// Verify bold sections
	if !strings.Contains(html, "🔥 <b>Executive Summary</b>") {
		t.Errorf("expected '🔥 <b>Executive Summary</b>', got:\n%s", html)
	}
	if !strings.Contains(html, "🚀 <b>Релизы и инструменты</b>") {
		t.Errorf("expected '🚀 <b>Релизы и инструменты</b>', got:\n%s", html)
	}
	if !strings.Contains(html, "• <b>Anthropic представила модели Claude Fable 5.2 и Claude Mythos 5.2:</b>") {
		t.Errorf("expected bold headline in bullet, got:\n%s", html)
	}
	if !strings.Contains(html, "<i>Warhammer 40,000: Dark Heresy</i>") {
		t.Errorf("expected italic game title, got:\n%s", html)
	}
	if !strings.Contains(html, "<i>Death Stranding Isolations</i>") {
		t.Errorf("expected italic anime title, got:\n%s", html)
	}
}

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

func TestSplitDigestIntoChannelMessages(t *testing.T) {
	// Case 1: Multiple channel sections with headers
	md := `# 📰 Telegram Feed Digest

### 📢 [4PDA Community](https://t.me/community4pda) (@community4pda)
• **Item A1:** Details about Item A1
• **Item A2:** Details about Item A2

---

### 📢 [Golang Weekly](https://t.me/golang_weekly) (@golang_weekly)
• **Item B1:** Details about Item B1`

	sections := SplitDigestIntoChannelMessages(md)
	if len(sections) != 2 {
		t.Fatalf("expected 2 channel sections, got %d: %v", len(sections), sections)
	}

	if !strings.Contains(sections[0], "4PDA Community") || !strings.Contains(sections[0], "Item A1") {
		t.Errorf("section 0 missing channel 1 content: %s", sections[0])
	}
	if !strings.Contains(sections[1], "Golang Weekly") || !strings.Contains(sections[1], "Item B1") {
		t.Errorf("section 1 missing channel 2 content: %s", sections[1])
	}

	// Case 2: Multi-channel with substantive Executive Summary overview
	mdOverview := `# 📰 Telegram Feed Digest

🔥 **Executive Summary**
Major breakthroughs across AI and cloud computing happened today.
Here is the high-level synthesis for engineering leads.

---

### 📢 4PDA Community (@community4pda)
• **Item 1:** Test 1

---

### 📢 Golang Weekly (@golang_weekly)
• **Item 2:** Test 2`

	overviewSections := SplitDigestIntoChannelMessages(mdOverview)
	if len(overviewSections) != 3 {
		t.Fatalf("expected 3 sections (overview + 2 channels), got %d", len(overviewSections))
	}
	if !strings.Contains(overviewSections[0], "Executive Summary") {
		t.Errorf("section 0 should be overview, got: %s", overviewSections[0])
	}
	if !strings.Contains(overviewSections[1], "4PDA Community") {
		t.Errorf("section 1 should be 4PDA, got: %s", overviewSections[1])
	}
	if !strings.Contains(overviewSections[2], "Golang Weekly") {
		t.Errorf("section 2 should be Golang Weekly, got: %s", overviewSections[2])
	}

	// Case 3: Single channel digest
	mdSingle := `### 📢 4PDA Community (@community4pda)
• **Post 1:** Content 1
• **Post 2:** Content 2`

	singleSections := SplitDigestIntoChannelMessages(mdSingle)
	if len(singleSections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(singleSections))
	}
}
