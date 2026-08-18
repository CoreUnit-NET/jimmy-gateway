package chatjimmy

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var statsPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?s)<\|stats\|>(.*?)<\|/stats\|>`),
	regexp.MustCompile(`(?s)<stats>(.*?)</stats>`),
}

type ParsedUpstream struct {
	Text  string
	Usage Usage
	Stats map[string]any
}

func ParseUpstream(raw string) ParsedUpstream {
	text := raw
	statsRaw := ""
	for _, pattern := range statsPatterns {
		match := pattern.FindStringSubmatch(text)
		if match == nil {
			continue
		}
		statsRaw = strings.TrimSpace(match[1])
		text = strings.TrimSpace(pattern.ReplaceAllString(text, ""))
		break
	}

	usage := Usage{}
	stats := map[string]any{}
	if statsRaw != "" {
		if err := json.Unmarshal([]byte(statsRaw), &stats); err != nil {
			stats = map[string]any{"raw": statsRaw}
		} else {
			usage.PromptTokens = safeInt(stats["prefill_tokens"])
			usage.CompletionTokens = safeInt(stats["decode_tokens"])
			usage.TotalTokens = safeInt(stats["total_tokens"])
			if usage.TotalTokens == 0 {
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			}
		}
	}

	return ParsedUpstream{
		Text:  strings.TrimSpace(text),
		Usage: usage,
		Stats: stats,
	}
}

func safeInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(v)
		return i
	default:
		return 0
	}
}
