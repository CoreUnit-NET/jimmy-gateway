package chatjimmy

import (
	"sort"
	"strings"
)

// ModelAliases maps OpenAI / Anthropic / Gemini client ids onto ChatJimmy's
// single upstream model. Unknown names pass through unchanged.
var ModelAliases = map[string]string{
	"gpt-4o":                     DefaultModel,
	"gpt-4o-mini":                DefaultModel,
	"gpt-4-turbo":                DefaultModel,
	"gpt-3.5-turbo":              DefaultModel,
	"gpt-4":                      DefaultModel,
	"gpt-3.5-turbo-instruct":     DefaultModel,
	"claude-opus-4-5-20251101":   DefaultModel,
	"claude-sonnet-4-5-20250929": DefaultModel,
	"claude-3-5-haiku-20241022":  DefaultModel,
	"claude-3-opus-20240229":     DefaultModel,
	"claude-3-sonnet-20240229":   DefaultModel,
	"claude-3-haiku-20240307":    DefaultModel,
	"gemini-1.5-pro":             DefaultModel,
	"gemini-1.5-flash":           DefaultModel,
	"gemini-1.0-pro":             DefaultModel,
	"gemini-1.0-ultra":           DefaultModel,
}

func MapModel(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultModel
	}
	if mapped, ok := ModelAliases[name]; ok {
		return mapped
	}
	return name
}

func ListedModels() []string {
	return ListModels("", nil)
}

func ListModels(defaultModel string, extras []string) []string {
	defaultModel = strings.TrimSpace(defaultModel)
	if defaultModel == "" {
		defaultModel = DefaultModel
	}

	seen := map[string]struct{}{defaultModel: {}}
	out := []string{defaultModel}

	aliases := make([]string, 0, len(ModelAliases))
	for alias := range ModelAliases {
		if alias == defaultModel {
			continue
		}
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		out = append(out, alias)
	}
	for _, extra := range extras {
		extra = strings.TrimSpace(extra)
		if extra == "" {
			continue
		}
		if _, ok := seen[extra]; ok {
			continue
		}
		seen[extra] = struct{}{}
		out = append(out, extra)
	}
	return out
}

func SplitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
