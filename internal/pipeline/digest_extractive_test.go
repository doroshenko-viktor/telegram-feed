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
