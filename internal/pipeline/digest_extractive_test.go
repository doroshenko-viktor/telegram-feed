package pipeline

import (
	"strings"
	"testing"
	"time"

	"telegram_feed_summarizer/internal/domain"
)

func TestGenerateExtractiveDigest(t *testing.T) {
	now := time.Now()
	channels := []*domain.Channel{
		{
			ID:       100,
			Title:    "Go News",
			Username: "golang_news",
		},
		{
			ID:       200,
			Title:    "Kubernetes Daily",
			Username: "k8s_daily",
		},
	}

	posts := []*domain.Post{
		{
			ChannelID: 100,
			MsgID:     1,
			Date:      now.Add(-2 * time.Hour),
			Text:      "Go 1.25 Released!\nThis version improves runtime latency and adds new standard library packages. Source code at https://github.com/golang/go and docs at https://golang.org/doc.\n#golang #release",
			Views:     5000,
			Forwards:  120,
		},
		{
			ChannelID: 100,
			MsgID:     2,
			Date:      now.Add(-1 * time.Hour),
			Text:      "Quick tip: use sync.Pool for buffer recycling.\nBenchmark results showed 40% memory reduction.",
			Views:     2000,
			Forwards:  45,
		},
		{
			ChannelID: 200,
			MsgID:     10,
			Date:      now.Add(-30 * time.Minute),
			Text:      "Kubernetes 1.34 Alpha is out with dynamic resource allocation updates.\nПодписывайтесь на наш канал @k8s_daily!",
			Views:     3500,
			Forwards:  80,
		},
	}

	digest, err := GenerateExtractiveDigest("tech", posts, channels)
	if err != nil {
		t.Fatalf("GenerateExtractiveDigest failed: %v", err)
	}

	if digest == nil {
		t.Fatal("expected non-nil digest")
	}

	if digest.PostCount != 3 {
		t.Errorf("expected PostCount 3, got %d", digest.PostCount)
	}

	if len(digest.ChannelIDs) != 2 {
		t.Errorf("expected 2 channel IDs, got %d", len(digest.ChannelIDs))
	}

	md := digest.SummaryMarkdown

	// 1. Verify channel headers
	if !strings.Contains(md, "Go News") {
		t.Errorf("expected markdown to contain 'Go News', got:\n%s", md)
	}
	if !strings.Contains(md, "Kubernetes Daily") {
		t.Errorf("expected markdown to contain 'Kubernetes Daily', got:\n%s", md)
	}

	// 2. Verify complete sentences and headline
	if !strings.Contains(md, "Go 1.25 Released!") {
		t.Errorf("expected markdown to contain post headline 'Go 1.25 Released!', got:\n%s", md)
	}
	if !strings.Contains(md, "This version improves runtime latency and adds new standard library packages.") {
		t.Errorf("expected markdown to contain complete sentence, got:\n%s", md)
	}

	// 3. Verify external resource extraction & GitHub detection
	if !strings.Contains(md, "[GitHub: golang/go](https://github.com/golang/go)") {
		t.Errorf("expected GitHub repo link, got:\n%s", md)
	}
	if !strings.Contains(md, "golang.org/doc") {
		t.Errorf("expected documentation link, got:\n%s", md)
	}

	// 4. Verify post link
	if !strings.Contains(md, "https://t.me/golang_news/1") {
		t.Errorf("expected markdown to contain link to post 1, got:\n%s", md)
	}

	// 5. CRITICAL: Verify metric clutter is completely eliminated
	if strings.Contains(md, "views") || strings.Contains(md, "👁️") || strings.Contains(md, "fwd") {
		t.Errorf("markdown must NOT contain view/forward metric noise, but got:\n%s", md)
	}

	// 6. Verify promotional lines and hashtags are stripped
	if strings.Contains(md, "Подписывайтесь") {
		t.Errorf("promotional subscription line was not filtered out:\n%s", md)
	}
	if strings.Contains(md, "#golang") {
		t.Errorf("hashtag line was not filtered out:\n%s", md)
	}
}

func TestGenerateExtractiveDigest_RussianAndBullets(t *testing.T) {
	now := time.Now()
	channels := []*domain.Channel{
		{
			ID:       300,
			Title:    "ClickHouse Ru",
			Username: "clickhouse_ru",
		},
	}

	posts := []*domain.Post{
		{
			ChannelID: 300,
			MsgID:     45,
			Date:      now.Add(-10 * time.Minute),
			Text: "Релиз clickhouse-backup 2.6.0!\n" +
				"Команда выпустила свежий релиз инструмента для резервного копирования ClickHouse. Основное новшество — поддержка инкрементальных бэкапов в S3 без блокировки таблиц.\n" +
				"Что нового:\n" +
				"- Поддержка многопоточной выгрузки чанков\n" +
				"- Снижение нагрузки на диск на 30%\n" +
				"- Исправление бага с удалением старых реплик\n" +
				"Репозиторий проекта: https://github.com/Altinity/clickhouse-backup\n" +
				"Документация: https://altinity.com/docs/clickhouse-backup\n" +
				"Подписывайтесь на наш канал @clickhouse_ru\n" +
				"#clickhouse #backup #release",
			Views:    8000,
			Forwards: 250,
		},
	}

	digest, err := GenerateExtractiveDigest("clickhouse", posts, channels)
	if err != nil {
		t.Fatalf("GenerateExtractiveDigest failed: %v", err)
	}

	md := digest.SummaryMarkdown

	if !strings.Contains(md, "Релиз clickhouse-backup 2.6.0") {
		t.Errorf("expected headline to be preserved, got:\n%s", md)
	}
	if !strings.Contains(md, "Поддержка многопоточной выгрузки чанков") {
		t.Errorf("expected bullet highlight to be preserved, got:\n%s", md)
	}
	if !strings.Contains(md, "[GitHub: Altinity/clickhouse-backup](https://github.com/Altinity/clickhouse-backup)") {
		t.Errorf("expected GitHub repo link, got:\n%s", md)
	}
	if !strings.Contains(md, "altinity.com/docs/clickhouse-backup") {
		t.Errorf("expected documentation link, got:\n%s", md)
	}
	if strings.Contains(md, "views") || strings.Contains(md, "👁️") || strings.Contains(md, "fwd") {
		t.Errorf("engagement metrics found in output:\n%s", md)
	}
}

func TestSplitSentences(t *testing.T) {
	text := "Go 1.25 is out! This improves latency. Visit https://example.com for details. Final sentence."
	sentences := splitSentences(text)
	if len(sentences) != 4 {
		t.Fatalf("expected 4 sentences, got %d: %#v", len(sentences), sentences)
	}
	if sentences[0] != "Go 1.25 is out!" {
		t.Errorf("sentence 0 mismatch: %s", sentences[0])
	}
	if sentences[1] != "This improves latency." {
		t.Errorf("sentence 1 mismatch: %s", sentences[1])
	}
	if sentences[2] != "Visit https://example.com for details." {
		t.Errorf("sentence 2 mismatch: %s", sentences[2])
	}
	if sentences[3] != "Final sentence." {
		t.Errorf("sentence 3 mismatch: %s", sentences[3])
	}
}

func TestExtractLinks(t *testing.T) {
	text := "Check [Release Notes](https://example.com/notes) and repo https://github.com/gin-gonic/gin. Also ignore t.me/+invite123 and t.me/joinchat/xyz."
	links := extractLinks(text)

	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d: %#v", len(links), links)
	}

	// GitHub repo should be first
	if !links[0].IsGitHub || links[0].Label != "GitHub: gin-gonic/gin" {
		t.Errorf("expected GitHub link first, got: %#v", links[0])
	}
	if links[1].Label != "Release Notes" || links[1].URL != "https://example.com/notes" {
		t.Errorf("expected markdown link second, got: %#v", links[1])
	}
}

func TestGenerateExtractiveDigest_Empty(t *testing.T) {
	_, err := GenerateExtractiveDigest("all", nil, nil)
	if err == nil {
		t.Fatal("expected error for empty posts, got nil")
	}
}

func TestGenerateExtractiveDigest_NoiseAndGossipScrubbing(t *testing.T) {
	now := time.Now()
	channels := []*domain.Channel{
		{
			ID:       400,
			Title:    "4PDA Community",
			Username: "", // Test channel without public username (private / nameless)
		},
	}

	posts := []*domain.Post{
		{
			ChannelID: 400,
			MsgID:     101,
			Date:      now.Add(-6 * time.Hour),
			Text: "Как устроен интернет — теперь можно изучить на одном сайте\n\n" +
				"Нашли интерактивный учебник, который наглядно разбирает работу 14 сетевых протоколов. " +
				"На примерах объясняют, как работает TCP и каким образом DNS находит нужный сайт в сети.\n\n" +
				"Ссылка: [ЗДЕСЬ](https://how-internet-works.com)\n\n" +
				"❤️ PDA",
			Views:    15000,
			Forwards: 420,
		},
		{
			ChannelID: 400,
			MsgID:     102,
			Date:      now.Add(-5 * time.Hour),
			Text: "Дженсен Хуанг изменил жене с китайской шпионкой — самый главный скандал в ИИ-сфере сгущается тучами вокруг главы Nvidia.\n" +
				"Всё началось с расследования журналистов Financial Times. Со временем частые встречи переросли в предполагаемый роман.\n" +
				"❤️ PDA",
			Views:    50000,
			Forwards: 2500, // Very viral gossip, would normally rank #1 by engagement
		},
		{
			ChannelID: 400,
			MsgID:     103,
			Date:      now.Add(-4 * time.Hour),
			Text: "Предзагрузка GTA 6 СТАРТОВАЛА — если у вас новозеландский регион в PSN, игру уже можно скачать.\n" +
				"Для других стран предзагрузку подвезут позже:\n" +
				"— Индия: 21:30 по Москве;\n" +
				"Владельцы Xbox Series смогут загрузить игру в 03:00 по МСК 10 ноября вне зависимости от региона.",
			Views:    30000,
			Forwards: 1200,
		},
		{
			ChannelID: 400,
			MsgID:     104,
			Date:      now.Add(-3 * time.Hour),
			Text: "GTA 6 можно будет запустить на взломанной PS5 — новая версия джейлбрейка для версии 14.22 выйдет уже ЗАВТРА\n" +
				"Пару часов назад стало известно, что именно эта версия прошивки поддалась взлому.\n" +
				"❤️ PDA",
			Views:    35000,
			Forwards: 1500,
		},
		{
			ChannelID: 400,
			MsgID:     105,
			Date:      now.Add(-3 * time.Hour),
			Text: "Baldur's Gate теперь Warhammer — новая Warhammer 40,000: Dark Heresy выйдет 28 января Это большая cRPG про расследования в космосе.\n" +
				"Вы будете работать на Инквизицию: расследовать заговоры, допрашивать свидетелей и искать врагов Империума.",
			Views:    18000,
			Forwards: 500,
		},
		{
			ChannelID: 400,
			MsgID:     106,
			Date:      now.Add(-2 * time.Hour),
			Text: "Нормана Ридуса не узнать: Death Stranding получит аниме-адаптацию Kojima Productions показала первый тизер сериала Death Stranding Isolations.\n" +
				"События развернутся вскоре после первой игры.",
			Views:    14000,
			Forwards: 400,
		},
		{
			ChannelID: 400,
			MsgID:     107,
			Date:      now.Add(-2 * time.Hour),
			Text: "Дрозофилу заставили 31 час подряд смотреть уроки по C++, чтобы превратить обычную муху в настоящего программиста.\n" +
				"Теперь осталось выяснить, научилась ли она писать код.\n" +
				"❤️ PDA",
			Views:    28000,
			Forwards: 900,
		},
		{
			ChannelID: 400,
			MsgID:     108,
			Date:      now.Add(-1 * time.Hour),
			Text: "В Телеграм могут появится сердечки вместо кружочков — команда мессенджера показала концепт других форматов для видеосообщений.\n" +
				"❤️ PDA",
			Views:    12000,
			Forwards: 300,
		},
		{
			ChannelID: 400,
			MsgID:     109,
			Date:      now.Add(-30 * time.Minute),
			Text: "Anthropic выпустила сразу две новые нейросети — Claude Fable 5.2 и Claude Mythos 5.2.\n" +
				"По заявлению компании, обе модели превосходят Opus 5.5 по большинству показателей. ❤️ PDA\n" +
				"• Fable 5.2 генерирует ответы до 30% быстрее предшественницы.\n" +
				"• API стоит $8 за миллион входных и $40 за миллион выходных токенов.\n" +
				"• Mythos 5.2 пока доступна только организациям со специальным разрешением.\n" +
				"Официальный анонс: https://anthropic.com/news/claude-5-2",
			Views:    22000,
			Forwards: 1100,
		},
	}

	digest, err := GenerateExtractiveDigest("tech", posts, channels)
	if err != nil {
		t.Fatalf("GenerateExtractiveDigest failed: %v", err)
	}

	md := digest.SummaryMarkdown

	// 1. Actionable technical posts MUST be present
	if !strings.Contains(md, "- **Как устроен интернет — теперь можно изучить на одном сайте**") {
		t.Errorf("expected clean separate headline for internet post, got:\n%s", md)
	}
	if !strings.Contains(md, "Нашли интерактивный учебник, который наглядно разбирает работу 14 сетевых протоколов.") {
		t.Errorf("expected substantive body for internet post, got:\n%s", md)
	}
	// Verify generic label [ЗДЕСЬ] replaced with clean domain label
	if !strings.Contains(md, "[how-internet-works.com](https://how-internet-works.com)") {
		t.Errorf("expected clean domain link instead of [ЗДЕСЬ], got:\n%s", md)
	}
	// Verify Telegram Post link was generated for channel without username
	if !strings.Contains(md, "https://t.me/c/400/101") {
		t.Errorf("expected https://t.me/c/400/101 post link, got:\n%s", md)
	}
	if !strings.Contains(md, "Claude Fable 5.2 и Claude Mythos 5.2") {
		t.Errorf("expected AI breakthrough post to be included, got:\n%s", md)
	}
	if !strings.Contains(md, "anthropic.com/news/claude-5-2") {
		t.Errorf("expected Anthropic announcement link, got:\n%s", md)
	}

	// 2. Gossip, scandals, game preload schedules, console jailbreaks, and meme posts MUST be excluded
	if strings.Contains(md, "Дженсен Хуанг") || strings.Contains(md, "изменил жене") {
		t.Errorf("gossip/scandal post was NOT filtered out:\n%s", md)
	}
	if strings.Contains(md, "Предзагрузка GTA 6") || strings.Contains(md, "новозеландский регион") {
		t.Errorf("game preload schedule was NOT filtered out:\n%s", md)
	}
	if strings.Contains(md, "джейлбрейка") || strings.Contains(md, "взломанной PS5") {
		t.Errorf("PS5 console jailbreak post was NOT filtered out:\n%s", md)
	}
	if strings.Contains(md, "Baldur's Gate теперь Warhammer") || strings.Contains(md, "Dark Heresy") {
		t.Errorf("Warhammer game release post was NOT filtered out:\n%s", md)
	}
	if strings.Contains(md, "Нормана Ридуса") || strings.Contains(md, "Death Stranding") {
		t.Errorf("Death Stranding anime post was NOT filtered out:\n%s", md)
	}
	if strings.Contains(md, "Дрозофилу") || strings.Contains(md, "муху в настоящего программиста") {
		t.Errorf("meme post was NOT filtered out:\n%s", md)
	}
	if strings.Contains(md, "сердечки вместо кружочков") {
		t.Errorf("trivial cosmetic rumor was NOT filtered out:\n%s", md)
	}

	// 3. Channel watermarks/signatures like "❤️ PDA" MUST be stripped
	if strings.Contains(md, "❤️ PDA") || strings.Contains(md, "❤️ 4PDA") {
		t.Errorf("channel watermark '❤️ PDA' was NOT stripped:\n%s", md)
	}

	// 4. Verification of counts: 7 noise posts were filtered out. Must NOT say "... and 7 more posts in this channel"
	if strings.Contains(md, "more posts in this channel") {
		t.Errorf("erroneous 'more posts in this channel' was output when all substantive posts were displayed:\n%s", md)
	}
	if !strings.Contains(md, "(7 noise/fluff posts filtered out by relevance criteria)") {
		t.Errorf("expected explicit notification about 7 filtered noise posts, got:\n%s", md)
	}
}
