package pipeline

import (
	"strings"

	"telegram_feed_summarizer/internal/domain"
)

// Standard topic dictionaries for heuristic fallback when no LLM API key is present
var knownTechTopics = map[string][]string{
	"go":         {"golang", "go programming", "goroutine", "go 1."},
	"kubernetes": {"k8s", "kubernetes", "kubectl", "helm"},
	"devops":     {"devops", "ci/cd", "terraform", "ansible", "docker", "sre"},
	"databases":  {"postgres", "postgresql", "mysql", "redis", "mongodb", "clickhouse", "sqlite"},
	"python":     {"python", "django", "fastapi", "flask", "pydantic"},
	"rust":       {"rustlang", "rust", "cargo"},
	"security":   {"security", "infosec", "vulnerability", "cve", "cybersecurity", "безопасность"},
	"ai":         {"machine learning", "deep learning", "llm", "neural network", "chatgpt", "openai"},
	"linux":      {"linux", "kernel", "systemd", "bash", "ubuntu", "debian"},
}

// HeuristicProfileChannel extracts topic tags from title, bio, and sample posts without an LLM API key.
func HeuristicProfileChannel(ch *domain.Channel, samplePosts []string) (summary string, topics []string, contentType string) {
	combined := strings.ToLower(ch.Title + " " + ch.About + " " + strings.Join(samplePosts, " "))

	matchedTopics := make(map[string]bool)
	for topic, keywords := range knownTechTopics {
		for _, kw := range keywords {
			if strings.Contains(combined, kw) {
				matchedTopics[topic] = true
				break
			}
		}
	}

	for t := range matchedTopics {
		topics = append(topics, t)
	}
	if len(topics) == 0 {
		topics = []string{"general"}
	}

	summary = ch.About
	if summary == "" {
		summary = ch.Title
	}

	contentType = "news"
	return summary, topics, contentType
}
