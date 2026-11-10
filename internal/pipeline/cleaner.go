package pipeline

import (
	"regexp"
	"strings"
	"sync"

	"telegram_feed_summarizer/internal/domain"
)

// RegexCache caches compiled regular expressions to avoid recompilation overhead.
var (
	regexCache = make(map[string]*regexp.Regexp)
	regexMu    sync.RWMutex
)

func getCompiledRegex(pattern string) *regexp.Regexp {
	regexMu.RLock()
	re, exists := regexCache[pattern]
	regexMu.RUnlock()
	if exists {
		return re
	}

	regexMu.Lock()
	defer regexMu.Unlock()
	// Double check
	if re, exists := regexCache[pattern]; exists {
		return re
	}

	compiled, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil
	}
	regexCache[pattern] = compiled
	return compiled
}

// Cleaner evaluates messages against dynamic database-stored ad patterns.
type Cleaner struct {
	patterns []*domain.AdPattern
}

// NewCleaner creates a cleaner initialized with ad patterns loaded from the database.
func NewCleaner(patterns []*domain.AdPattern) *Cleaner {
	return &Cleaner{patterns: patterns}
}

// IsAdvertisement checks if a text matches any active database ad pattern.
func (c *Cleaner) IsAdvertisement(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || c == nil {
		return false
	}
	lower := strings.ToLower(trimmed)

	for _, p := range c.patterns {
		if !p.Enabled {
			continue
		}

		if p.IsRegex {
			re := getCompiledRegex(p.Pattern)
			if re != nil && re.MatchString(trimmed) {
				return true
			}
		} else {
			// Substring / keyword match (case-insensitive)
			if strings.Contains(lower, strings.ToLower(p.Pattern)) {
				return true
			}
		}
	}

	return false
}
