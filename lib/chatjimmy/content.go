package chatjimmy

import (
	"encoding/json"
	"strings"
)

func ContentToText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err == nil {
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(part, &obj); err != nil {
				out = append(out, string(part))
				continue
			}
			if t, ok := obj["text"]; ok {
				out = append(out, ContentToText(t))
				continue
			}
			out = append(out, string(part))
		}
		return strings.Join(out, "\n")
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		if t, ok := obj["text"]; ok {
			return ContentToText(t)
		}
		if c, ok := obj["content"]; ok {
			return ContentToText(c)
		}
	}

	return string(raw)
}
