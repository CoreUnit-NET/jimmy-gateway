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
	aliases := make([]string, 0, len(ModelAliases))
	for alias := range ModelAliases {
		if alias == DefaultModel {
			continue
		}
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return append([]string{DefaultModel}, aliases...)
}
