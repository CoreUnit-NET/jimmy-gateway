package chatjimmy

import (
	"bytes"
	"encoding/json"
)

func isEmptyJSON(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

// MarshalJSON encodes v as JSON without HTML escaping.
// OpenAI-compatible clients expect literal <, >, and & in string values.
func MarshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func mustJSON(v any) json.RawMessage {
	b, err := MarshalJSON(v)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return b
}
